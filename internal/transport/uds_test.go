package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
)

func shortTempSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "xctl-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "ctl.sock")
	if len(sock) > 100 {
		dir2, err2 := os.MkdirTemp("/tmp", "xctl-")
		if err2 != nil {
			t.Fatalf("MkdirTemp(/tmp): %v", err2)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir2) })
		sock = filepath.Join(dir2, "ctl.sock")
	}
	return sock
}

func startTestUDSServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	sock := shortTempSocketPath(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sock, err)
	}
	srv := &http.Server{Handler: handler}
	go func() {
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return sock
}

func TestFromEnvValidation(t *testing.T) {
	if _, err := FromEnv(func(string) string { return "" }); err == nil {
		t.Fatal("expected error when XLYRA_CTL_SOCK is empty")
	}
	if _, err := FromEnv(func(k string) string {
		if k == EnvCtlSock {
			return "relative/ctl.sock"
		}
		return ""
	}); err == nil {
		t.Fatal("expected error when XLYRA_CTL_SOCK is relative")
	}

	regularFile := filepath.Join(t.TempDir(), "not-a-sock")
	if err := os.WriteFile(regularFile, []byte("hi"), 0o600); err != nil {
		t.Fatalf("write regular file: %v", err)
	}
	if _, err := FromEnv(func(k string) string {
		if k == EnvCtlSock {
			return regularFile
		}
		return ""
	}); err == nil {
		t.Fatal("expected error when XLYRA_CTL_SOCK points to a regular file")
	}
}

func TestClientCallSuccessAndHeaders(t *testing.T) {
	var gotProto, gotNonce string
	var gotReq protocol.CallRequest

	sock := startTestUDSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/call" || r.Method != http.MethodPost {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		gotProto = r.Header.Get(protocol.HeaderProtocol)
		gotNonce = r.Header.Get(protocol.HeaderNonce)
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(protocol.Envelope{
			OK:   true,
			Data: map[string]any{"status": "ready"},
			Meta: &protocol.Meta{Tier: 1},
		})
	}))

	cfg, err := FromEnv(func(k string) string {
		switch k {
		case EnvCtlSock:
			return sock
		case EnvCtlNonce:
			return "nonce-123"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}

	client := NewClient(cfg)
	defer client.Close()

	status, env, err := client.Call(context.Background(), protocol.CallRequest{
		Resource: "status",
		Action:   "get",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if status != http.StatusOK || !env.OK {
		t.Fatalf("unexpected response: status=%d env=%+v", status, env)
	}
	if gotProto != "1" || gotNonce != "nonce-123" {
		t.Fatalf("unexpected headers: proto=%q nonce=%q", gotProto, gotNonce)
	}
	if gotReq.Protocol != 1 || gotReq.Resource != "status" || gotReq.Action != "get" {
		t.Fatalf("unexpected request: %+v", gotReq)
	}
}

func TestClientCallNon200EnvelopeAndCancellation(t *testing.T) {
	started := make(chan struct{})
	sock := startTestUDSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req protocol.CallRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Action == "hang" {
			close(started)
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(protocol.NewErrorEnvelope("policy_denied", "forbidden", "agent.settings", "web only"))
	}))

	client := NewClient(Config{SocketPath: sock})
	defer client.Close()

	status, env, err := client.Call(context.Background(), protocol.CallRequest{Resource: "config", Action: "set"})
	if err != nil {
		t.Fatalf("Call non-200: %v", err)
	}
	if status != http.StatusForbidden || env.OK || env.Error == nil || env.Error.Code != "policy_denied" {
		t.Fatalf("unexpected error envelope: status=%d env=%+v", status, env)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, _, callErr := client.Call(ctx, protocol.CallRequest{Resource: "site", Action: "hang"})
		errCh <- callErr
	}()
	<-started
	cancel()
	callErr := <-errCh
	var trErr *Error
	if !errors.As(callErr, &trErr) || trErr.Code != "cancelled" {
		t.Fatalf("expected cancelled transport error, got %v", callErr)
	}
}

func TestClientRejectsOversizedResponse(t *testing.T) {
	sock := startTestUDSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":"` + strings.Repeat("a", maxResponseBytes+10) + `"}`))
	}))

	client := NewClient(Config{SocketPath: sock})
	defer client.Close()

	_, _, err := client.Call(context.Background(), protocol.CallRequest{Resource: "status", Action: "get"})
	var trErr *Error
	if !errors.As(err, &trErr) || trErr.Code != "ctl_invalid_response" {
		t.Fatalf("expected ctl_invalid_response, got %v", err)
	}
}
