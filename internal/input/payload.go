package input

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxPayloadBytes = 1 << 20 // 1 MiB limit for --file and --stdin JSON payloads

// LoadPayload reads and decodes JSON payload from --json, --file, or --stdin.
// Returns (decodedValue, hasPayload, error).
func LoadPayload(g GlobalFlags, stdin io.Reader) (any, bool, error) {
	sources := 0
	if g.JSON != "" {
		sources++
	}
	if g.File != "" {
		sources++
	}
	if g.Stdin {
		sources++
	}
	if sources == 0 {
		return nil, false, nil
	}
	if sources > 1 {
		return nil, false, &ValidationError{
			Code:    "invalid_argument",
			Message: "--json、--file 与 --stdin 只能同时指定一个",
		}
	}

	var raw []byte
	var err error
	switch {
	case g.JSON != "":
		raw = []byte(g.JSON)
		if len(raw) > maxPayloadBytes {
			return nil, false, &ValidationError{
				Code:    "invalid_argument",
				Message: "--json 载荷超过 1 MiB 上限",
			}
		}
	case g.File != "":
		raw, err = readWorkspaceFile(g.File)
		if err != nil {
			return nil, false, err
		}
	case g.Stdin:
		if stdin == nil {
			return nil, false, &ValidationError{Code: "invalid_argument", Message: "标准输入不可用"}
		}
		raw, err = io.ReadAll(io.LimitReader(stdin, maxPayloadBytes+1))
		if err != nil {
			return nil, false, &ValidationError{Code: "invalid_argument", Message: fmt.Sprintf("读取 stdin 失败: %v", err)}
		}
		if len(raw) > maxPayloadBytes {
			return nil, false, &ValidationError{Code: "invalid_argument", Message: "stdin 载荷超过 1 MiB 上限"}
		}
	}

	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false, &ValidationError{
			Code:    "invalid_argument",
			Message: "JSON 载荷不能为空",
		}
	}

	var decoded any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return nil, false, &ValidationError{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("JSON 载荷解析失败: %v", err),
		}
	}
	// Ensure no trailing garbage after the first JSON value.
	if dec.More() {
		return nil, false, &ValidationError{Code: "invalid_argument", Message: "JSON 载荷包含多余数据"}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, false, &ValidationError{Code: "invalid_argument", Message: "JSON 载荷只能包含单个 JSON 值"}
	}

	return decoded, true, nil
}

// readWorkspaceFile enforces cwd-confined realpath checks on --file so localBash
// cannot read arbitrary host files outside the active workspace.
func readWorkspaceFile(rawPath string) ([]byte, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return nil, &ValidationError{Code: "invalid_argument", Message: "--file 路径不能为空", Field: "file"}
	}
	if strings.HasPrefix(trimmed, "~") || strings.HasPrefix(trimmed, "file://") {
		return nil, &ValidationError{
			Code:    "policy_denied",
			Message: "--file 不允许使用 ~ 或 file:// 路径，只能读取当前工作区内的文件",
			Field:   "file",
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	realCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve real working directory: %w", err)
	}

	abs := trimmed
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, abs)
	}
	abs = filepath.Clean(abs)

	realTarget, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, &ValidationError{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("无法访问文件 %q: %v", rawPath, err),
			Field:   "file",
		}
	}

	if realTarget == realCwd || !isInsideDir(realCwd, realTarget) {
		return nil, &ValidationError{
			Code:    "policy_denied",
			Message: "--file 路径不在当前会话工作目录内，已拒绝访问",
			Field:   "file",
			Hint:    "请仅传入当前工作区相对路径或工作区内的文件",
		}
	}

	f, err := os.Open(realTarget)
	if err != nil {
		return nil, &ValidationError{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("打开文件失败: %v", err),
			Field:   "file",
		}
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, &ValidationError{Code: "invalid_argument", Message: fmt.Sprintf("读取文件状态失败: %v", err), Field: "file"}
	}
	if !info.Mode().IsRegular() {
		return nil, &ValidationError{Code: "invalid_argument", Message: "--file 只能读取普通文件", Field: "file"}
	}
	if info.Size() > maxPayloadBytes {
		return nil, &ValidationError{Code: "invalid_argument", Message: "--file 文件大小超过 1 MiB 上限", Field: "file"}
	}

	data, err := io.ReadAll(io.LimitReader(f, maxPayloadBytes+1))
	if err != nil {
		return nil, &ValidationError{Code: "invalid_argument", Message: fmt.Sprintf("读取文件失败: %v", err), Field: "file"}
	}
	if len(data) > maxPayloadBytes {
		return nil, &ValidationError{Code: "invalid_argument", Message: "--file 文件大小超过 1 MiB 上限", Field: "file"}
	}
	return data, nil
}

func isInsideDir(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if child == parent {
		return true
	}
	prefix := parent
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(child, prefix)
}

// ValidateProxyURLNoPassword rejects proxy URLs that embed a password (Tier 0 policy).
func ValidateProxyURLNoPassword(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return &ValidationError{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("代理 URL 非法: %v", err),
			Field:   "url",
		}
	}
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			return &ValidationError{
				Code:    "policy_denied",
				Message: "一期禁止通过 xlyra-cli 在代理 URL 中传入密码（Tier 0）",
				Field:   "url",
				Hint:    "请在 Web 控制台创建或修改带密码的网络代理",
			}
		}
	}
	return nil
}

// ValidateNoSensitiveRequestHeaders rejects site payloads attempting to write auth headers via CLI.
func ValidateNoSensitiveRequestHeaders(payload any) error {
	obj, ok := payload.(map[string]any)
	if !ok || obj == nil {
		return nil
	}
	rawHeaders, exists := obj["request_headers"]
	if !exists || rawHeaders == nil {
		return nil
	}

	switch headers := rawHeaders.(type) {
	case map[string]any:
		for name := range headers {
			if isSensitiveHeaderName(name) {
				return sensitiveHeaderError(name)
			}
		}
	case []any:
		for _, item := range headers {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"name", "key", "header"} {
				if name, ok := entry[key].(string); ok && isSensitiveHeaderName(name) {
					return sensitiveHeaderError(name)
				}
			}
		}
	}
	return nil
}

func isSensitiveHeaderName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	switch lower {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "api-key", "x-auth-token", "x-access-token":
		return true
	}
	for _, needle := range []string{"authorization", "api-key", "apikey", "secret", "password", "passwd", "token", "cookie", "credential"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func sensitiveHeaderError(name string) error {
	return &ValidationError{
		Code:    "policy_denied",
		Message: fmt.Sprintf("一期禁止通过 xlyra-cli 写入敏感鉴权请求头 %q（Tier 0）", name),
		Field:   "request_headers",
		Hint:    "请在 Web 控制台的站点设置中配置含密钥的请求头",
	}
}
