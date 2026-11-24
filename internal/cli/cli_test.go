package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
	"github.com/Yachiiiiyo/xLyra-cli/internal/transport"
)

type recordedCall struct {
	Req   protocol.CallRequest
	Nonce string
}

type mockResponse struct {
	Status   int
	Envelope protocol.Envelope
}

func startMockUDSServer(t *testing.T, handler func(req protocol.CallRequest, r *http.Request) mockResponse) (string, *[]recordedCall) {
	t.Helper()
	dir, err := os.MkdirTemp("", "xcli-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "ctl.sock")
	if len(sock) > 100 {
		dir2, err2 := os.MkdirTemp("/tmp", "xcli-")
		if err2 != nil {
			t.Fatalf("MkdirTemp(/tmp): %v", err2)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir2) })
		sock = filepath.Join(dir2, "ctl.sock")
	}

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sock, err)
	}

	var mu sync.Mutex
	var calls []recordedCall
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/call" || r.Method != http.MethodPost {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			var req protocol.CallRequest
			dec := json.NewDecoder(r.Body)
			dec.UseNumber()
			if err := dec.Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			calls = append(calls, recordedCall{
				Req:   req,
				Nonce: r.Header.Get(protocol.HeaderNonce),
			})
			mu.Unlock()

			res := handler(req, r)
			if res.Status == 0 {
				res.Status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(res.Status)
			_ = json.NewEncoder(w).Encode(res.Envelope)
		}),
	}
	go func() {
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return sock, &calls
}

func runCLIWithEnv(t *testing.T, sock, nonce, stdinData string, args ...string) (int, string, protocol.Envelope) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := NewApp()
	app.Stdout = &stdout
	app.Stderr = &stderr
	if stdinData != "" {
		app.Stdin = strings.NewReader(stdinData)
	}
	app.EnvLookup = func(key string) string {
		switch key {
		case transport.EnvCtlSock:
			return sock
		case transport.EnvCtlNonce:
			return nonce
		default:
			return ""
		}
	}

	code := app.Run(context.Background(), args)
	var env protocol.Envelope
	raw := bytes.TrimSpace(stdout.Bytes())
	if len(raw) > 0 && raw[0] == '{' {
		_ = json.Unmarshal(raw, &env)
	}
	return code, stdout.String(), env
}

func TestVersionAndHelpWithoutSocket(t *testing.T) {
	for _, flag := range []string{"version", "--version", "-v"} {
		code, out, env := runCLIWithEnv(t, "", "", "", flag)
		if code != protocol.ExitOK || !env.OK {
			t.Fatalf("%s failed: code=%d out=%s", flag, code, out)
		}
		data, _ := env.Data.(map[string]any)
		if data["protocol_version"] != float64(1) {
			t.Fatalf("expected protocol_version 1, got %v", data["protocol_version"])
		}
	}

	for _, helpArgs := range [][]string{nil, {"--help"}, {"help"}, {"help", "help"}, {"help", "site"}, {"site", "--help"}} {
		code, out, _ := runCLIWithEnv(t, "", "", "", helpArgs...)
		if code != protocol.ExitOK || !strings.Contains(out, "xlyra") {
			t.Fatalf("help %v failed: code=%d out=%s", helpArgs, code, out)
		}
	}

	code, _, env := runCLIWithEnv(t, "", "", "", "status")
	if code != protocol.ExitInternal || env.OK || env.Error == nil || env.Error.Code != "ctl_unavailable" {
		t.Fatalf("expected ctl_unavailable with exit 1, got code=%d env=%+v", code, env)
	}
}

func TestAll14DomainRoutesAndOptions(t *testing.T) {
	sock, calls := startMockUDSServer(t, func(req protocol.CallRequest, _ *http.Request) mockResponse {
		return mockResponse{
			Status: http.StatusOK,
			Envelope: protocol.Envelope{
				OK: true,
				Data: map[string]any{
					"resource": req.Resource,
					"action":   req.Action,
					"secret":   "should_be_projected_out",
				},
				Meta: &protocol.Meta{Tier: 1, DryRun: req.Options.DryRun},
			},
		}
	})

	cases := []struct {
		args      []string
		resource  string
		action    string
		target    string
		subTarget string
	}{
		// A & B
		{[]string{"status"}, "status", "get", "", ""},
		{[]string{"schema"}, "schema", "list", "", ""},
		{[]string{"schema", "site"}, "schema", "get", "site", ""},
		{[]string{"config", "list"}, "config", "list", "", ""},
		{[]string{"config", "describe", "general.log.level"}, "config", "describe", "general.log.level", ""},
		{[]string{"config", "get", "general"}, "config", "get", "general", ""},
		{[]string{"config", "set", "general.log.level", "debug", "--dry-run"}, "config", "set", "general.log.level", ""},
		{[]string{"config", "set", "general.log.level=debug", "general.cache.enabled=false"}, "config", "set", "", ""},
		{[]string{"config", "patch", "general", "--json", `{"log":{"level":"warn"}}`, "--if-match", "sha256:1"}, "config", "patch", "general", ""},
		{[]string{"config", "reset", "general.log.level"}, "config", "reset", "general.log.level", ""},

		// C & D
		{[]string{"proxy", "list"}, "proxy", "list", "", ""},
		{[]string{"proxy", "get", "p1"}, "proxy", "get", "p1", ""},
		{[]string{"proxy", "create", "--name", "hk", "--type", "socks5", "--url", "socks5://127.0.0.1:1080"}, "proxy", "create", "", ""},
		{[]string{"proxy", "update", "p1", "--name", "hk2"}, "proxy", "update", "p1", ""},
		{[]string{"proxy", "delete", "p1"}, "proxy", "delete", "p1", ""},
		{[]string{"proxy", "test", "--type", "http", "--url", "http://127.0.0.1:7890"}, "proxy", "test", "", ""},
		{[]string{"site", "list", "--enabled", "true", "--oauth", "exclude", "--type", "openai"}, "site", "list", "", ""},
		{[]string{"site", "get", "openrouter"}, "site", "get", "openrouter", ""},
		{[]string{"site", "create", "--json", `{"name":"or","slug":"openrouter"}`}, "site", "create", "", ""},
		{[]string{"site", "update", "openrouter", "--json", `{"routing_priority":5}`}, "site", "update", "openrouter", ""},
		{[]string{"site", "enable", "openrouter"}, "site", "enable", "openrouter", ""},
		{[]string{"site", "disable", "openrouter"}, "site", "disable", "openrouter", ""},
		{[]string{"site", "set-gateway", "openrouter", "max_concurrency=32", "impersonate_codex_client=true"}, "site", "set-gateway", "openrouter", ""},
		{[]string{"site", "set-plugin-order", "openrouter", "p1,p2"}, "site", "set-plugin-order", "openrouter", ""},
		{[]string{"site", "delete", "openrouter"}, "site", "delete", "openrouter", ""},
		{[]string{"site", "refresh", "--all"}, "site", "refresh", "", ""},
		{[]string{"site", "health", "openrouter", "--history", "--check"}, "site", "health", "openrouter", ""},
		{[]string{"site", "sync-models", "openrouter"}, "site", "sync-models", "openrouter", ""},
		{[]string{"site-group", "create", "--name", "primary", "--site-ids", "s1,s2"}, "site-group", "create", "", ""},
		{[]string{"site-key", "list", "openrouter"}, "site-key", "list", "openrouter", ""},
		{[]string{"site-key", "update", "openrouter", "k1", "--enabled", "false", "--priority", "10"}, "site-key", "update", "openrouter", "k1"},
		{[]string{"site-key", "set-model", "openrouter", "k1", "--model", "gpt-5", "--enabled", "true"}, "site-key", "set-model", "openrouter", "k1"},
		{[]string{"site-key", "delete", "openrouter", "k1"}, "site-key", "delete", "openrouter", "k1"},
		{[]string{"site-account", "list", "openrouter"}, "site-account", "list", "openrouter", ""},
		{[]string{"site-account", "update", "openrouter", "c1", "--enabled", "false"}, "site-account", "update", "openrouter", "c1"},
		{[]string{"site-account", "set-model", "openrouter", "c1", "--model", "claude", "--enabled", "true"}, "site-account", "set-model", "openrouter", "c1"},
		{[]string{"site-account", "refresh", "openrouter", "--all"}, "site-account", "refresh", "openrouter", ""},
		{[]string{"oauth", "list"}, "oauth", "list", "", ""},
		{[]string{"oauth", "get", "conn-1"}, "oauth", "get", "conn-1", ""},
		{[]string{"oauth", "set-model", "conn-1", "--site-model-id", "sm-1", "--enabled", "true"}, "oauth", "set-model", "conn-1", ""},
		{[]string{"oauth", "refresh", "conn-1"}, "oauth", "refresh", "conn-1", ""},

		// E
		{[]string{"model", "list", "--provider", "openai", "--status", "active", "--page", "2", "--page-size", "25"}, "model", "list", "", ""},
		{[]string{"model", "get", "gpt-4o", "--matrix"}, "model", "get", "gpt-4o", ""},
		{[]string{"model", "merge", "deepseek-v4.1-flash", "deepseek-flash"}, "model", "merge", "deepseek-v4.1-flash", ""},
		{[]string{"model", "alias", "add", "gpt-4o", "chatgpt-4o"}, "model", "alias-add", "gpt-4o", ""},
		{[]string{"model", "alias", "delete", "gpt-4o", "a1"}, "model", "alias-delete", "gpt-4o", "a1"},
		{[]string{"model", "pricing", "set", "gpt-4o", "--input", "2.5", "--output", "10"}, "model", "pricing-set", "gpt-4o", ""},
		{[]string{"model", "pricing", "reset", "gpt-4o"}, "model", "pricing-reset", "gpt-4o", ""},
		{[]string{"site-model", "list", "--site", "openrouter", "--canonical", "deepseek-flash", "--enabled", "true", "--search", "flash", "--page", "2", "--page-size", "20"}, "site-model", "list", "", ""},
		{[]string{"site-model", "create", "openrouter", "--upstream-name", "openai/gpt-4o", "--endpoints", "chat,responses"}, "site-model", "create", "openrouter", ""},
		{[]string{"site-model", "batch-status", "openrouter", "--ids", "m1,m2", "--enabled", "false"}, "site-model", "batch-status", "openrouter", ""},
		{[]string{"site-model", "bind-canonical", "sm1", "none"}, "site-model", "bind-canonical", "sm1", ""},
		{[]string{"site-model", "test", "openrouter", "sm1", "--protocol", "openai"}, "site-model", "test", "openrouter", "sm1"},
		{[]string{"pricing", "list", "--site", "openrouter"}, "pricing", "list", "", ""},
		{[]string{"pricing", "set", "sm1", "--json", `{"input_value":1.5}`}, "pricing", "set", "sm1", ""},
		{[]string{"pricing", "bulk-set", "--json", `{"items":[]}`}, "pricing", "bulk-set", "", ""},
		{[]string{"pricing", "reset", "--site", "openrouter"}, "pricing", "reset", "", ""},

		// F & G
		{[]string{"api-key", "list"}, "api-key", "list", "", ""},
		{[]string{"api-key", "create", "--name", "ci-key"}, "api-key", "create", "", ""},
		{[]string{"api-key", "reorder", "k1,k2"}, "api-key", "reorder", "", ""},
		{[]string{"api-key", "set-sites", "k1", "--policy", "allow_list", "--ids", "s1"}, "api-key", "set-sites", "k1", ""},
		{[]string{"api-key", "set-site-groups", "k1", "--ids", "g1"}, "api-key", "set-site-groups", "k1", ""},
		{[]string{"api-key", "set-site-models", "k1", "--policy", "allow_all"}, "api-key", "set-site-models", "k1", ""},
		{[]string{"api-key", "set-mappings", "k1", "--json", `[{"pattern":"a","target":"b"}]`}, "api-key", "set-mappings", "k1", ""},
		{[]string{"api-key", "quota", "increase", "k1", "--amount", "50"}, "api-key", "quota-increase", "k1", ""},
		{[]string{"api-key", "quota", "reset", "k1"}, "api-key", "quota-reset", "k1", ""},
		{[]string{"api-key", "check-model", "k1", "gpt-4o"}, "api-key", "check-model", "k1", "gpt-4o"},
		{[]string{"api-key", "rotate", "k1"}, "api-key", "rotate", "k1", ""},
		{[]string{"route", "overview", "--window-hours", "12"}, "route", "overview", "", ""},
		{[]string{"route", "candidates", "gpt-4o", "--limit", "10", "--debug"}, "route", "candidates", "gpt-4o", ""},
		{[]string{"route", "select", "--model", "gpt-4o", "--api-key", "k1", "--debug"}, "route", "select", "", ""},
		{[]string{"route", "failover", "--model", "gpt-4o", "--failed-site-ids", "s1,s2"}, "route", "failover", "", ""},
		{[]string{"route", "traces", "--model", "gpt-4o", "--limit", "10"}, "route", "traces", "", ""},
		{[]string{"cooldown", "list"}, "cooldown", "list", "", ""},
		{[]string{"cooldown", "create", "--site", "openrouter", "--duration", "300", "--reason", "429"}, "cooldown", "create", "", ""},
		{[]string{"cooldown", "clear", "--site", "openrouter"}, "cooldown", "clear", "", ""},

		// H
		{[]string{"plugin", "list"}, "plugin", "list", "", ""},
		{[]string{"plugin", "settings", "get", "plg-1"}, "plugin", "settings-get", "plg-1", ""},
		{[]string{"plugin", "settings", "set", "plg-1", "--json", `{"k":"v"}`}, "plugin", "settings-set", "plg-1", ""},
		{[]string{"plugin", "site-settings", "set", "plg-1", "openrouter", "--json", `{"k":"v"}`}, "plugin", "site-settings-set", "plg-1", "openrouter"},
		{[]string{"plugin", "schedule", "set", "plg-1", "--enabled", "true"}, "plugin", "schedule-set", "plg-1", ""},
		{[]string{"plugin", "subscription", "set", "plg-1", "--auto-update", "true"}, "plugin", "subscription-set", "plg-1", ""},
		{[]string{"plugin", "automation", "update", "plg-1", "auto-1", "--json", `{"inputs":{}}`}, "plugin", "automation-update", "plg-1", "auto-1"},
		{[]string{"plugin", "bind-site", "plg-1", "openrouter"}, "plugin", "bind-site", "plg-1", "openrouter"},
		{[]string{"plugin", "bind-protocol", "plg-1", "openai"}, "plugin", "bind-protocol", "plg-1", "openai"},
		{[]string{"plugin", "enable", "plg-1", "1.0.0", "--grant-permissions", "http", "--confirm-hosts", "api.example.com"}, "plugin", "enable", "plg-1", "1.0.0"},
		{[]string{"plugin", "trusted-keys", "list"}, "plugin", "trusted-keys-list", "", ""},
		{[]string{"plugin-market", "install", "plg-1", "--version", "1.0.0"}, "plugin-market", "install", "plg-1", ""},

		// I & J
		{[]string{"agents-md", "get"}, "agents-md", "get", "", ""},
		{[]string{"skill", "list"}, "skill", "list", "", ""},
		{[]string{"skill", "get", "my-skill"}, "skill", "get", "my-skill", ""},
		{[]string{"backup", "sections"}, "backup", "sections", "", ""},
		{[]string{"backup", "auto-run"}, "backup", "auto-run", "", ""},
		{[]string{"cloud", "status"}, "cloud", "status", "", ""},
		{[]string{"dashboard", "overview", "--window-hours", "24"}, "dashboard", "overview", "", ""},
		{[]string{"dashboard", "insights"}, "dashboard", "insights", "", ""},
		{[]string{"request", "list", "--site", "openrouter", "--days", "3", "--limit", "20", "--page", "1"}, "request", "list", "", ""},
		{[]string{"request", "get", "req-123"}, "request", "get", "req-123", ""},
		{[]string{"request", "summary", "--days", "7"}, "request", "summary", "", ""},
		{[]string{"request", "channel-split", "--days", "7"}, "request", "channel-split", "", ""},
		{[]string{"usage", "summary", "--days", "7"}, "usage", "summary", "", ""},
		{[]string{"usage", "api-keys", "--days", "7"}, "usage", "api-keys", "", ""},
	}

	for _, tc := range cases {
		code, out, env := runCLIWithEnv(t, sock, "nonce-ok", "", tc.args...)
		if code != protocol.ExitOK || !env.OK {
			t.Fatalf("command %v failed: code=%d out=%s", tc.args, code, out)
		}
		last := (*calls)[len(*calls)-1]
		if last.Nonce != "nonce-ok" {
			t.Fatalf("command %v missing nonce, got %q", tc.args, last.Nonce)
		}
		if last.Req.Resource != tc.resource || last.Req.Action != tc.action || last.Req.Target != tc.target || last.Req.SubTarget != tc.subTarget {
			t.Fatalf("command %v routed to (%q,%q,%q,%q), want (%q,%q,%q,%q)",
				tc.args,
				last.Req.Resource, last.Req.Action, last.Req.Target, last.Req.SubTarget,
				tc.resource, tc.action, tc.target, tc.subTarget)
		}
	}

	// Verify --fields projection and --format table/yaml integration
	code, _, env := runCLIWithEnv(t, sock, "nonce-ok", "", "status", "--fields", "resource")
	if code != protocol.ExitOK {
		t.Fatalf("fields projection failed: code=%d", code)
	}
	data := env.Data.(map[string]any)
	if _, hasSecret := data["secret"]; hasSecret {
		t.Fatal("expected --fields projection to drop unrequested fields")
	}

	code, yamlOut, _ := runCLIWithEnv(t, sock, "nonce-ok", "", "status", "--format", "yaml")
	if code != protocol.ExitOK || !strings.Contains(yamlOut, "ok: true") {
		t.Fatalf("yaml format failed: code=%d out=%s", code, yamlOut)
	}

	code, tableOut, _ := runCLIWithEnv(t, sock, "nonce-ok", "", "status", "--format", "table")
	if code != protocol.ExitOK || !strings.Contains(tableOut, "KEY") {
		t.Fatalf("table format failed: code=%d out=%s", code, tableOut)
	}
}

func TestExitCodeMappings(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		code     string
		wantExit int
	}{
		{"ok", http.StatusOK, "", protocol.ExitOK},
		{"invalid_400", http.StatusBadRequest, "validation_error", protocol.ExitInvalid},
		{"not_found_404", http.StatusNotFound, "not_found", protocol.ExitNotFound},
		{"policy_denied_403", http.StatusForbidden, "policy_denied", protocol.ExitPolicyDenied},
		{"rate_limited_429", http.StatusTooManyRequests, "rate_limited", protocol.ExitPolicyDenied},
		{"approval_denied", http.StatusForbidden, "approval_denied", protocol.ExitApprovalDenied},
		{"approval_timeout", http.StatusRequestTimeout, "approval_timeout", protocol.ExitApprovalDenied},
		{"conflict_409", http.StatusConflict, "revision_mismatch", protocol.ExitConflict},
		{"internal_500", http.StatusInternalServerError, "internal_error", protocol.ExitInternal},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sock, _ := startMockUDSServer(t, func(_ protocol.CallRequest, _ *http.Request) mockResponse {
				if tc.status == http.StatusOK {
					return mockResponse{Status: http.StatusOK, Envelope: protocol.NewSuccessEnvelope(map[string]any{"ok": true})}
				}
				return mockResponse{
					Status:   tc.status,
					Envelope: protocol.NewErrorEnvelope(tc.code, "test error", "", ""),
				}
			})
			got, out, _ := runCLIWithEnv(t, sock, "", "", "status")
			if got != tc.wantExit {
				t.Fatalf("want exit %d, got %d (out=%s)", tc.wantExit, got, out)
			}
		})
	}
}

func TestClientSidePolicyAndWorkspaceBlocks(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "escape.json")
	if err := os.WriteFile(outsideFile, []byte(`{"x":1}`), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(ws); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	// No UDS server should even be needed for client-side blocks.
	blockedCases := [][]string{
		{"site-key", "create", "openrouter", "--name", "k"},
		{"proxy", "create", "--name", "p", "--type", "http", "--url", "http://u:p@host:8080"},
		{"proxy", "create", "--json", `{"name":"p","type":"http","url":"http://u:p@host:8080"}`},
		{"site", "create", "--json", `{"name":"or","request_headers":[{"name":"Authorization","value":"Bearer x"}]}`},
		{"site", "update", "openrouter", "--json", `{"request_headers":{"X-API-Key":"secret"}}`},
		{"site", "create", "--file", "../" + filepath.Base(outside) + "/escape.json"},
		{"backup", "restore"},
		{"backup", "import"},
		{"agents-md", "set"},
		{"skill", "put"},
		{"plugin", "trusted-keys", "add"},
	}

	for _, args := range blockedCases {
		code, out, env := runCLIWithEnv(t, "", "", "", args...)
		if code != protocol.ExitPolicyDenied || env.OK || env.Error == nil || env.Error.Code != "policy_denied" {
			t.Fatalf("expected exit 4 policy_denied for %v, got code=%d out=%s", args, code, out)
		}
	}
}

func TestCatalogAndRouteNewFlagsAndPayloads(t *testing.T) {
	sock, calls := startMockUDSServer(t, func(req protocol.CallRequest, _ *http.Request) mockResponse {
		return mockResponse{
			Status:   http.StatusOK,
			Envelope: protocol.NewSuccessEnvelope(map[string]any{"ok": true}),
		}
	})

	// 1. model merge
	code, out, env := runCLIWithEnv(t, sock, "n1", "", "model", "merge", "deepseek-v4.1-flash", "deepseek-flash")
	if code != protocol.ExitOK || !env.OK {
		t.Fatalf("model merge failed: code=%d out=%s", code, out)
	}
	last := (*calls)[len(*calls)-1].Req
	payload, _ := last.Payload.(map[string]any)
	if last.Target != "deepseek-v4.1-flash" || payload["target_model_id"] != "deepseek-flash" {
		t.Fatalf("unexpected model merge req: %+v", last)
	}

	// 2. model list pagination & status
	code, out, env = runCLIWithEnv(t, sock, "n1", "", "model", "list", "--provider", "deepseek", "--status", "active", "--page", "2", "--page-size", "15")
	if code != protocol.ExitOK || !env.OK {
		t.Fatalf("model list failed: code=%d out=%s", code, out)
	}
	last = (*calls)[len(*calls)-1].Req
	if last.Params["provider"] != "deepseek" || last.Params["status"] != "active" ||
		last.Params["page"] != json.Number("2") || last.Params["page_size"] != json.Number("15") {
		t.Fatalf("unexpected model list params: %#v", last.Params)
	}

	// 3. site-model list filters & pagination
	code, out, env = runCLIWithEnv(t, sock, "n1", "", "site-model", "list", "--site", "openrouter", "--canonical", "deepseek-flash", "--enabled", "true", "--search", "flash", "--page", "2", "--page-size", "20")
	if code != protocol.ExitOK || !env.OK {
		t.Fatalf("site-model list failed: code=%d out=%s", code, out)
	}
	last = (*calls)[len(*calls)-1].Req
	if last.Params["site"] != "openrouter" || last.Params["canonical"] != "deepseek-flash" ||
		last.Params["enabled"] != true || last.Params["search"] != "flash" ||
		last.Params["page"] != json.Number("2") || last.Params["page_size"] != json.Number("20") {
		t.Fatalf("unexpected site-model list params: %#v", last.Params)
	}

	// 4. route candidates --limit & --debug
	code, out, env = runCLIWithEnv(t, sock, "n1", "", "route", "candidates", "deepseek-flash", "--limit", "5", "--debug")
	if code != protocol.ExitOK || !env.OK {
		t.Fatalf("route candidates failed: code=%d out=%s", code, out)
	}
	last = (*calls)[len(*calls)-1].Req
	if last.Target != "deepseek-flash" || last.Params["limit"] != json.Number("5") || last.Params["debug"] != true {
		t.Fatalf("unexpected route candidates req: %+v", last)
	}
}
