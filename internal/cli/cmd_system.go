package cli

import (
	"context"
	"fmt"

	"github.com/Yachiiiiyo/xLyra-cli/internal/input"
	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
)

func (a *App) runAgentsMD(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli agents-md get`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	switch sub {
	case "get":
		parsed, ok, code := a.parse(args[1:], nil, help)
		if !ok {
			return code
		}
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "agents-md get 不接受位置参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "agents-md", Action: "get"})
	case "set", "put", "update", "delete":
		return a.failPolicy(
			extractRequestedFormat(args[1:]),
			"AGENTS.md 属于 Agent 自身配置（Tier 0），禁止通过 xlyra-cli 修改",
			"agents-md",
			"请让管理员在 Web 控制台的 Agent 设置中修改",
		)
	default:
		return a.failInvalid(extractRequestedFormat(args[1:]), "invalid_argument", fmt.Sprintf("未知 agents-md 子命令: %s", sub), "", help)
	}
}

func (a *App) runSkill(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli skill list
  xlyra-cli skill get <name>`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	switch sub {
	case "list":
		parsed, ok, code := a.parse(args[1:], nil, help)
		if !ok {
			return code
		}
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "skill list 不接受位置参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "skill", Action: "list"})
	case "get":
		parsed, ok, code := a.parse(args[1:], nil, help)
		if !ok {
			return code
		}
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "skill get 需要 1 个 <name> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "skill", Action: "get", Target: parsed.Positionals[0]})
	case "put", "create", "set", "update", "delete":
		return a.failPolicy(
			extractRequestedFormat(args[1:]),
			"skills/* 属于 Agent 自身配置（Tier 0），禁止通过 xlyra-cli 修改",
			"skills",
			"请让管理员在 Web 控制台的 Agent 设置中管理技能",
		)
	default:
		return a.failInvalid(extractRequestedFormat(args[1:]), "invalid_argument", fmt.Sprintf("未知 skill 子命令: %s", sub), "", help)
	}
}

func (a *App) runBackup(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli backup sections
  xlyra-cli backup auto-test
  xlyra-cli backup auto-files
  xlyra-cli backup auto-run`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	switch sub {
	case "sections", "auto-test", "auto-files", "auto-run":
		parsed, ok, code := a.parse(args[1:], nil, help)
		if !ok {
			return code
		}
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("backup %s 不接受位置参数", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "backup", Action: sub})
	case "restore", "import":
		return a.failPolicy(
			extractRequestedFormat(args[1:]),
			"破坏性备份恢复属于严禁区（Tier 0），禁止 Agent 执行",
			"backup",
			"请在停止 Agent 后由管理员在 Web 控制台执行恢复",
		)
	default:
		return a.failInvalid(extractRequestedFormat(args[1:]), "invalid_argument", fmt.Sprintf("未知 backup 子命令: %s", sub), "", help)
	}
}

func (a *App) runCloud(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli cloud status
  xlyra-cli cloud report-preview`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	parsed, ok, code := a.parse(args[1:], nil, help)
	if !ok {
		return code
	}

	switch sub {
	case "status", "report-preview":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("cloud %s 不接受位置参数", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "cloud", Action: sub})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 cloud 子命令: %s", sub), "", help)
	}
}

func (a *App) runDashboard(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli dashboard overview [--window-hours 24]
  xlyra-cli dashboard insights [--window-hours 24]`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{{Name: "window-hours", Kind: input.FlagInt}}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "overview", "insights":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("dashboard %s 不接受位置参数", sub), "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.Int("window-hours"); exists {
			params["window_hours"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "dashboard", Action: sub, Params: params})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 dashboard 子命令: %s", sub), "", help)
	}
}

func (a *App) runRequest(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli request list [--site S] [--model M] [--status S] [--days 1..7] [--limit 1..50] [--page P]
  xlyra-cli request get <request-id>
  xlyra-cli request summary [--days 1..7]
  xlyra-cli request channel-split [--days 1..7]`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "site", Kind: input.FlagString},
		{Name: "model", Kind: input.FlagString},
		{Name: "status", Kind: input.FlagString},
		{Name: "days", Kind: input.FlagInt},
		{Name: "limit", Kind: input.FlagInt},
		{Name: "page", Kind: input.FlagInt},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "request list 不接受位置参数", "", "")
		}
		params := map[string]any{}
		for _, k := range []string{"site", "model", "status"} {
			if v, exists := parsed.String(k); exists {
				params[k] = v
			}
		}
		for _, k := range []string{"days", "limit", "page"} {
			if v, exists := parsed.Int(k); exists {
				params[k] = v
			}
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "request", Action: "list", Params: params})
	case "get":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "request get 需要 1 个 <request-id> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "request", Action: "get", Target: parsed.Positionals[0]})
	case "summary", "channel-split":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("request %s 不接受位置参数", sub), "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.Int("days"); exists {
			params["days"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "request", Action: sub, Params: params})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 request 子命令: %s", sub), "", help)
	}
}

func (a *App) runUsage(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli usage summary [--days 1..7]
  xlyra-cli usage api-keys [--days 1..7]`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{{Name: "days", Kind: input.FlagInt}}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "summary", "api-keys":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("usage %s 不接受位置参数", sub), "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.Int("days"); exists {
			params["days"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "usage", Action: sub, Params: params})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 usage 子命令: %s", sub), "", help)
	}
}
