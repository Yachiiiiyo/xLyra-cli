// Package transport implements the Control-Plane Unix Domain Socket client for xlyra-cli.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
	"github.com/Yachiiiiyo/xLyra-cli/internal/version"
)

const (
	// EnvCtlSock is the mandatory environment variable pointing to ctl.sock.
	EnvCtlSock = "XLYRA_CTL_SOCK"
	// EnvCtlNonce carries the per-run secret nonce used in non-isolated localBash mode.
	EnvCtlNonce = "XLYRA_CTL_NONCE"

	callEndpoint     = "http://ctl.local/v1/call"
	dialTimeout      = 5 * time.Second
	maxResponseBytes = 4 << 20 // 4 MiB safety ceiling against malformed payloads
)

// Config holds UDS connection settings resolved exclusively from the process environment.
type Config struct {
	SocketPath string
	Nonce      string
	UserAgent  string
}

// Error represents a structured local transport or environment validation failure.
type Error struct {
	Code    string
	Message string
	Hint    string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// Envelope converts the transport error into a standard CLI error envelope.
func (e *Error) Envelope() protocol.Envelope {
	return protocol.NewErrorEnvelope(e.Code, e.Message, "", e.Hint)
}

// FromEnv resolves and validates XLYRA_CTL_SOCK and XLYRA_CTL_NONCE from lookup.
// If lookup is nil, os.Getenv is used.
func FromEnv(lookup func(string) string) (Config, error) {
	if lookup == nil {
		lookup = os.Getenv
	}
	sock := strings.TrimSpace(lookup(EnvCtlSock))
	if sock == "" {
		return Config{}, &Error{
			Code:    "ctl_unavailable",
			Message: "缺少环境变量 XLYRA_CTL_SOCK，无法连接 xlyra-server 控制面",
			Hint:    "xlyra 仅限内置 Agent 会话内通过 Control-Plane UDS 运行，不支持外部 TCP/HTTP 连接",
		}
	}
	if !filepath.IsAbs(sock) {
		return Config{}, &Error{
			Code:    "ctl_unavailable",
			Message: fmt.Sprintf("XLYRA_CTL_SOCK 必须是绝对路径: %s", sock),
			Hint:    "请确认由 xlyra-server 注入了有效的 Unix Socket 路径",
		}
	}
	info, err := os.Stat(sock)
	if err != nil {
		return Config{}, &Error{
			Code:    "ctl_unavailable",
			Message: fmt.Sprintf("无法访问控制面套接字 %s", sock),
			Hint:    "请确认当前 Agent 会话处于活动状态且控制面套接字已挂载",
			Err:     err,
		}
	}
	if info.Mode()&os.ModeSocket == 0 {
		return Config{}, &Error{
			Code:    "ctl_unavailable",
			Message: fmt.Sprintf("XLYRA_CTL_SOCK 指向的不是 Unix Socket: %s", sock),
			Hint:    "请检查会话控制面配置",
		}
	}
	return Config{
		SocketPath: filepath.Clean(sock),
		Nonce:      strings.TrimSpace(lookup(EnvCtlNonce)),
		UserAgent:  "xlyra/" + version.Version,
	}, nil
}

// Client sends Control-Plane v1 requests over an AF_UNIX stream socket.
type Client struct {
	cfg        Config
	httpClient *http.Client
	transport  *http.Transport
}

// NewClient constructs a UDS HTTP/1.1 client for the validated Config.
func NewClient(cfg Config) *Client {
	dialer := &net.Dialer{Timeout: dialTimeout}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", cfg.SocketPath)
		},
		DisableKeepAlives: true,
	}
	return &Client{
		cfg:       cfg,
		transport: tr,
		// No client-wide timeout so blocking Tier 2/3 approval cards can pause cleanly
		// until either the user decides or ctx is cancelled.
		httpClient: &http.Client{Transport: tr},
	}
}

// Close closes any idle transport connections.
func (c *Client) Close() {
	if c != nil && c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}

// Call dispatches a single CallRequest over the UDS connection and returns the HTTP status and envelope.
func (c *Client) Call(ctx context.Context, req protocol.CallRequest) (int, protocol.Envelope, error) {
	if req.Protocol == 0 {
		req.Protocol = version.ProtocolVersion
	}
	body, err := json.Marshal(req)
	if err != nil {
		return 0, protocol.Envelope{}, &Error{
			Code:    "invalid_request",
			Message: "序列化控制面请求失败",
			Err:     err,
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, callEndpoint, bytes.NewReader(body))
	if err != nil {
		return 0, protocol.Envelope{}, &Error{
			Code:    "internal_error",
			Message: "构造控制面 HTTP 请求失败",
			Err:     err,
		}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set(protocol.HeaderProtocol, strconv.Itoa(version.ProtocolVersion))
	if c.cfg.Nonce != "" {
		httpReq.Header.Set(protocol.HeaderNonce, c.cfg.Nonce)
	}
	if c.cfg.UserAgent != "" {
		httpReq.Header.Set("User-Agent", c.cfg.UserAgent)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return 0, protocol.Envelope{}, &Error{
				Code:    "cancelled",
				Message: "控制面请求已取消",
				Err:     err,
			}
		}
		return 0, protocol.Envelope{}, &Error{
			Code:    "ctl_transport_error",
			Message: "连接 xlyra-server 控制面失败",
			Hint:    "请确认会话尚未结束且 ctl.sock 处于监听状态",
			Err:     err,
		}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return resp.StatusCode, protocol.Envelope{}, &Error{
			Code:    "ctl_transport_error",
			Message: "读取控制面响应失败",
			Err:     err,
		}
	}
	if len(raw) > maxResponseBytes {
		return resp.StatusCode, protocol.Envelope{}, &Error{
			Code:    "ctl_invalid_response",
			Message: "控制面响应体超过客户端安全上限",
		}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return resp.StatusCode, protocol.Envelope{}, &Error{
			Code:    "ctl_invalid_response",
			Message: fmt.Sprintf("控制面返回空响应体 (HTTP %d)", resp.StatusCode),
		}
	}

	var env protocol.Envelope
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&env); err != nil {
		return resp.StatusCode, protocol.Envelope{}, &Error{
			Code:    "ctl_invalid_response",
			Message: fmt.Sprintf("控制面返回非 JSON 响应 (HTTP %d)", resp.StatusCode),
			Err:     err,
		}
	}
	return resp.StatusCode, env, nil
}
