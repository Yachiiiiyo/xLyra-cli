// Package cli implements the xlyra-cli command router, help output, and UDS dispatch.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/Yachiiiiyo/xLyra-cli/internal/input"
	"github.com/Yachiiiiyo/xLyra-cli/internal/output"
	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
	"github.com/Yachiiiiyo/xLyra-cli/internal/transport"
	"github.com/Yachiiiiyo/xLyra-cli/internal/version"
)

// Client calls the xlyra-server control-plane UDS endpoint.
type Client interface {
	Call(ctx context.Context, req protocol.CallRequest) (int, protocol.Envelope, error)
	Close()
}

// ClientFactory builds a Client using the supplied environment lookup function.
type ClientFactory func(lookup func(string) string) (Client, error)

// App holds I/O streams and dependencies for xlyra-cli execution.
type App struct {
	Stdout        io.Writer
	Stderr        io.Writer
	Stdin         io.Reader
	EnvLookup     func(string) string
	ClientFactory ClientFactory
}

type handlerFunc func(ctx context.Context, args []string) int

// NewApp returns an App configured for production execution using stdio and UDS transport.
func NewApp() *App {
	return &App{
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Stdin:     os.Stdin,
		EnvLookup: os.Getenv,
		ClientFactory: func(lookup func(string) string) (Client, error) {
			cfg, err := transport.FromEnv(lookup)
			if err != nil {
				return nil, err
			}
			return transport.NewClient(cfg), nil
		},
	}
}

// Run parses top-level command routing and returns a deterministic exit code (0–6).
func (a *App) Run(ctx context.Context, args []string) int {
	a.ensureDefaults()
	if len(args) == 0 {
		a.printRootHelp()
		return protocol.ExitOK
	}

	first := args[0]
	switch first {
	case "-h", "--help", "help":
		if len(args) > 1 && args[1] != "help" && !strings.HasPrefix(args[1], "-") {
			return a.printSubHelp(args[1])
		}
		a.printRootHelp()
		return protocol.ExitOK
	case "-v", "--version", "version":
		return a.runVersion(args[1:])
	}

	if strings.HasPrefix(first, "-") {
		return a.failInvalid("json", "invalid_argument", fmt.Sprintf("未知顶层参数: %s", first), "", "请先指定命令名或运行 xlyra-cli --help")
	}

	routes := map[string]handlerFunc{
		"status":        a.runStatus,
		"schema":        a.runSchema,
		"config":        a.runConfig,
		"proxy":         a.runProxy,
		"site":          a.runSite,
		"site-group":    a.runSiteGroup,
		"site-key":      a.runSiteKey,
		"site-account":  a.runSiteAccount,
		"oauth":         a.runOAuth,
		"model":         a.runModel,
		"site-model":    a.runSiteModel,
		"pricing":       a.runPricing,
		"api-key":       a.runAPIKey,
		"route":         a.runRoute,
		"cooldown":      a.runCooldown,
		"plugin":        a.runPlugin,
		"plugin-market": a.runPluginMarket,
		"agents-md":     a.runAgentsMD,
		"skill":         a.runSkill,
		"backup":        a.runBackup,
		"cloud":         a.runCloud,
		"dashboard":     a.runDashboard,
		"request":       a.runRequest,
		"usage":         a.runUsage,
	}

	fn, ok := routes[first]
	if !ok {
		return a.failInvalid("json", "invalid_argument", fmt.Sprintf("未知命令: %s", first), "", "运行 xlyra-cli --help 查看全部可用命令")
	}
	return fn(ctx, args[1:])
}

func (a *App) ensureDefaults() {
	if a.Stdout == nil {
		a.Stdout = os.Stdout
	}
	if a.Stderr == nil {
		a.Stderr = os.Stderr
	}
	if a.EnvLookup == nil {
		a.EnvLookup = os.Getenv
	}
	if a.ClientFactory == nil {
		a.ClientFactory = func(lookup func(string) string) (Client, error) {
			cfg, err := transport.FromEnv(lookup)
			if err != nil {
				return nil, err
			}
			return transport.NewClient(cfg), nil
		}
	}
}

func (a *App) runVersion(args []string) int {
	parsed, err := input.ParseArgs(args, nil)
	if err != nil {
		return a.handleInputError("json", err)
	}
	if parsed.Global.Help {
		_, _ = fmt.Fprintln(a.Stdout, "用法: xlyra-cli version [--format json|table|yaml]")
		return protocol.ExitOK
	}
	env := protocol.NewSuccessEnvelope(version.Info(runtime.GOOS, runtime.GOARCH))
	return a.renderEnvelope(parsed.Global.Format, parsed.Global.Fields, 0, env)
}

func (a *App) parse(args []string, specs []input.FlagSpec, helpText string) (input.ParsedArgs, bool, int) {
	parsed, err := input.ParseArgs(args, specs)
	if err != nil {
		return input.ParsedArgs{}, false, a.handleInputError(extractRequestedFormat(args), err)
	}
	if parsed.Global.Help {
		_, _ = fmt.Fprintln(a.Stdout, strings.TrimSpace(helpText))
		return input.ParsedArgs{}, false, protocol.ExitOK
	}
	return parsed, true, protocol.ExitOK
}

func (a *App) loadPayload(g input.GlobalFlags) (any, bool, int) {
	payload, has, err := input.LoadPayload(g, a.Stdin)
	if err != nil {
		return nil, false, a.handleInputError(g.Format, err)
	}
	return payload, has, protocol.ExitOK
}

func (a *App) dispatch(ctx context.Context, g input.GlobalFlags, req protocol.CallRequest) int {
	req.Protocol = version.ProtocolVersion
	req.Options = protocol.CallOptions{
		DryRun:  g.DryRun,
		Fields:  g.Fields,
		IfMatch: g.IfMatch,
	}

	client, err := a.ClientFactory(a.EnvLookup)
	if err != nil {
		return a.handleTransportError(g.Format, err)
	}
	defer client.Close()

	httpStatus, env, err := client.Call(ctx, req)
	if err != nil {
		return a.handleTransportError(g.Format, err)
	}
	return a.renderEnvelope(g.Format, g.Fields, httpStatus, env)
}

func (a *App) renderEnvelope(format string, fields []string, httpStatus int, env protocol.Envelope) int {
	projected := output.ProjectEnvelope(env, fields)
	if err := output.Render(a.Stdout, format, projected); err != nil {
		_, _ = fmt.Fprintf(a.Stderr, "render error: %v\n", err)
		return protocol.ExitInternal
	}
	return protocol.ExitCodeForResponse(httpStatus, &env)
}

func (a *App) handleInputError(format string, err error) int {
	var vErr *input.ValidationError
	if errors.As(err, &vErr) {
		return a.failInvalid(format, vErr.Code, vErr.Message, vErr.Field, vErr.Hint)
	}
	return a.failInvalid(format, "invalid_argument", err.Error(), "", "")
}

func (a *App) handleTransportError(format string, err error) int {
	var trErr *transport.Error
	var env protocol.Envelope
	if errors.As(err, &trErr) {
		env = trErr.Envelope()
	} else {
		env = protocol.NewErrorEnvelope("ctl_transport_error", err.Error(), "", "")
	}
	if renderErr := output.Render(a.Stdout, format, env); renderErr != nil {
		_, _ = fmt.Fprintf(a.Stderr, "render error: %v\n", renderErr)
	}
	return protocol.ExitInternal
}

func (a *App) failInvalid(format, code, message, field, hint string) int {
	if code == "" {
		code = "invalid_argument"
	}
	env := protocol.NewErrorEnvelope(code, message, field, hint)
	if err := output.Render(a.Stdout, format, env); err != nil {
		_, _ = fmt.Fprintf(a.Stderr, "render error: %v\n", err)
		return protocol.ExitInternal
	}
	if code == "policy_denied" {
		return protocol.ExitPolicyDenied
	}
	return protocol.ExitInvalid
}

func (a *App) failPolicy(format, message, field, hint string) int {
	return a.failInvalid(format, "policy_denied", message, field, hint)
}

func extractRequestedFormat(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--format" && i+1 < len(args) {
			f := strings.ToLower(strings.TrimSpace(args[i+1]))
			if f == "json" || f == "table" || f == "yaml" {
				return f
			}
		}
		if strings.HasPrefix(args[i], "--format=") {
			f := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(args[i], "--format=")))
			if f == "json" || f == "table" || f == "yaml" {
				return f
			}
		}
	}
	return "json"
}

func (a *App) printSubHelp(cmd string) int {
	return a.Run(context.Background(), []string{cmd, "--help"})
}

func (a *App) printRootHelp() {
	_, _ = fmt.Fprint(a.Stdout, `xlyra-cli - xLyra 内置 Agent 控制面命令行工具

用法:
  xlyra-cli <command> [subcommand] [flags]

全局参数:
  --format json|table|yaml   输出格式（默认 json）
  --fields f1,f2,...         投影返回字段
  --dry-run                  预演写操作并返回结构化 Diff（不落盘）
  --json '<payload>'         直接传入 JSON 请求体
  --file <path>              从当前工作目录内 JSON 文件读取请求体
  --stdin                    从标准输入读取 JSON 请求体
  --if-match <revision>      乐观锁修订号校验
  -h, --help                 查看帮助

命令域:
  version                    查看二进制版本与协议版本（无需控制面连接）
  status                     查看控制面连接状态、权限模式与单 run 配额
  schema [domain]            查看配置域 JSON Schema、默认值与安全等级
  config                     单例配置点路径读写 (list|describe|get|set|patch|reset)
  proxy                      网络代理管理 (list|get|create|update|delete|test)
  site                       上游站点配置与运维 (list|get|create|update|enable|disable|set-gateway|...)
  site-group                 站点分组管理 (list|get|create|update|delete)
  site-key                   站点上游 Key 元数据管理 (list|update|set-model|delete)
  site-account               站点账号池管理 (list|update|set-model|refresh)
  oauth                      OAuth 连接管理 (list|get|set-model|refresh)
  model                      规范模型与别名/默认定价 (list|get|create|update|delete|alias|pricing)
  site-model                 站点模型管理 (list|create|update|batch-status|bind-canonical|test|delete)
  pricing                    站点模型定价管理 (list|set|bulk-set|reset)
  api-key                    下游 API Key 管理 (list|get|create|update|reorder|set-*|quota|check-model|rotate|delete)
  route                      路由诊断与模拟 (overview|candidates|select|failover|traces)
  cooldown                   路由冷却管理 (list|create|clear)
  plugin                     JS 插件配置与绑定 (list|get|settings|site-settings|schedule|subscription|automation|bind-*|enable|disable|delete|trusted-keys)
  plugin-market              插件市场查询与安装 (status|list|get|sync|install)
  agents-md                  只读查看 AGENTS.md (get)
  skill                      只读查看启用技能 (list|get)
  backup                     非破坏性备份查询与触发 (sections|auto-test|auto-files|auto-run)
  cloud                      Cloud 绑定只读查询 (status|report-preview)
  dashboard                  运行态网关概览与洞察 (overview|insights)
  request                    运行态请求观测 (list|get|summary|channel-split)
  usage                      运行态用量与费用统计 (summary|api-keys)
`)
}
