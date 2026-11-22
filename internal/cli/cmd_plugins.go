package cli

import (
	"context"
	"fmt"

	"github.com/Yachiiiiyo/xLyra-cli/internal/input"
	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
)

func (a *App) runPlugin(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli plugin list
  xlyra-cli plugin get|disable <plugin-id>
  xlyra-cli plugin settings get <plugin-id>
  xlyra-cli plugin settings set <plugin-id> (--json '{...}' | --file <path> | --stdin)
  xlyra-cli plugin site-settings set <plugin-id> <site> (--json '{...}' | --file <path> | --stdin)
  xlyra-cli plugin schedule get|set <plugin-id> [--enabled true|false]
  xlyra-cli plugin subscription set <plugin-id> [--auto-update true|false | --enabled true|false | --json '{...}']
  xlyra-cli plugin automation list|create <plugin-id> [--json '{...}']
  xlyra-cli plugin automation update|delete <plugin-id> <automation-id> [--json '{...}']
  xlyra-cli plugin bind-site|unbind-site <plugin-id> <site>
  xlyra-cli plugin bind-protocol <plugin-id> <protocol>
  xlyra-cli plugin enable <plugin-id> <version> [--grant-permissions ...] [--confirm-hosts ...] [--confirm-untrusted]
  xlyra-cli plugin delete <plugin-id> [--version V]
  xlyra-cli plugin trusted-keys list`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	switch sub {
	case "settings":
		return a.runPluginSettings(ctx, args[1:], help)
	case "site-settings":
		return a.runPluginSiteSettings(ctx, args[1:], help)
	case "schedule":
		return a.runPluginSchedule(ctx, args[1:], help)
	case "subscription":
		return a.runPluginSubscription(ctx, args[1:], help)
	case "automation":
		return a.runPluginAutomation(ctx, args[1:], help)
	case "trusted-keys":
		return a.runPluginTrustedKeys(ctx, args[1:], help)
	}

	specs := []input.FlagSpec{
		{Name: "site", Kind: input.FlagString},
		{Name: "protocol", Kind: input.FlagString},
		{Name: "version", Kind: input.FlagString},
		{Name: "grant-permissions", Kind: input.FlagCSV},
		{Name: "confirm-hosts", Kind: input.FlagCSV},
		{Name: "confirm-untrusted", Kind: input.FlagBool},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin list 不接受位置参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "plugin", Action: "list"})
	case "get", "disable":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("plugin %s 需要 1 个 <plugin-id> 参数", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "plugin", Action: sub, Target: parsed.Positionals[0]})
	case "bind-site", "unbind-site":
		siteVal, hasSite := parsed.String("site")
		var pluginID, site string
		switch {
		case len(parsed.Positionals) == 2 && !hasSite:
			pluginID, site = parsed.Positionals[0], parsed.Positionals[1]
		case len(parsed.Positionals) == 1 && hasSite:
			pluginID, site = parsed.Positionals[0], siteVal
		default:
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("plugin %s 需要 <plugin-id> <site>", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "plugin",
			Action:    sub,
			Target:    pluginID,
			SubTarget: site,
		})
	case "bind-protocol":
		protoVal, hasProto := parsed.String("protocol")
		var pluginID, proto string
		switch {
		case len(parsed.Positionals) == 2 && !hasProto:
			pluginID, proto = parsed.Positionals[0], parsed.Positionals[1]
		case len(parsed.Positionals) == 1 && hasProto:
			pluginID, proto = parsed.Positionals[0], protoVal
		default:
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin bind-protocol 需要 <plugin-id> <protocol>", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "plugin",
			Action:    "bind-protocol",
			Target:    pluginID,
			SubTarget: proto,
		})
	case "enable":
		verVal, hasVer := parsed.String("version")
		var pluginID, versionStr string
		switch {
		case len(parsed.Positionals) == 2 && !hasVer:
			pluginID, versionStr = parsed.Positionals[0], parsed.Positionals[1]
		case len(parsed.Positionals) == 1 && hasVer:
			pluginID, versionStr = parsed.Positionals[0], verVal
		default:
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin enable 需要 <plugin-id> <version>", "", "")
		}
		payload := map[string]any{"version": versionStr}
		if v, exists := parsed.Strings("grant-permissions"); exists {
			payload["grant_permissions"] = v
		}
		if v, exists := parsed.Strings("confirm-hosts"); exists {
			payload["confirm_hosts"] = v
		}
		if v, exists := parsed.Bool("confirm-untrusted"); exists {
			payload["confirm_untrusted"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "plugin",
			Action:    "enable",
			Target:    pluginID,
			SubTarget: versionStr,
			Payload:   payload,
		})
	case "delete":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin delete 需要 1 个 <plugin-id> 参数", "", "")
		}
		req := protocol.CallRequest{Resource: "plugin", Action: "delete", Target: parsed.Positionals[0]}
		if v, exists := parsed.String("version"); exists {
			req.SubTarget = v
			req.Params = map[string]any{"version": v}
		}
		return a.dispatch(ctx, parsed.Global, req)
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 plugin 子命令: %s", sub), "", help)
	}
}

func (a *App) runPluginSettings(ctx context.Context, args []string, help string) int {
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
	case "get":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin settings get 需要 1 个 <plugin-id> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "plugin",
			Action:   "settings-get",
			Target:   parsed.Positionals[0],
		})
	case "set":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin settings set 需要 1 个 <plugin-id> 参数", "", "")
		}
		payload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		if !hasPayload {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin settings set 需要 --json、--file 或 --stdin 载荷", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "plugin",
			Action:   "settings-set",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 plugin settings 子命令: %s", sub), "", help)
	}
}

func (a *App) runPluginSiteSettings(ctx context.Context, args []string, help string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}
	if args[0] != "set" {
		return a.failInvalid(extractRequestedFormat(args), "invalid_argument", fmt.Sprintf("未知 plugin site-settings 子命令: %s", args[0]), "", help)
	}
	parsed, ok, code := a.parse(args[1:], nil, help)
	if !ok {
		return code
	}
	if len(parsed.Positionals) != 2 {
		return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin site-settings set 需要 <plugin-id> <site>", "", "")
	}
	payload, hasPayload, pCode := a.loadPayload(parsed.Global)
	if pCode != protocol.ExitOK {
		return pCode
	}
	if !hasPayload {
		return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin site-settings set 需要 --json、--file 或 --stdin 载荷", "", "")
	}
	return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
		Resource:  "plugin",
		Action:    "site-settings-set",
		Target:    parsed.Positionals[0],
		SubTarget: parsed.Positionals[1],
		Payload:   payload,
	})
}

func (a *App) runPluginSchedule(ctx context.Context, args []string, help string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}
	sub := args[0]
	specs := []input.FlagSpec{{Name: "enabled", Kind: input.FlagBool}}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}
	switch sub {
	case "get":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin schedule get 需要 1 个 <plugin-id> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "plugin",
			Action:   "schedule-get",
			Target:   parsed.Positionals[0],
		})
	case "set":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin schedule set 需要 1 个 <plugin-id> 参数", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		if v, exists := parsed.Bool("enabled"); exists {
			overrides["enabled"] = v
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if _, hasEnabled := payload["enabled"]; !hasEnabled {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin schedule set 需要 --enabled true|false 或 --json", "enabled", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "plugin",
			Action:   "schedule-set",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 plugin schedule 子命令: %s", sub), "", help)
	}
}

func (a *App) runPluginSubscription(ctx context.Context, args []string, help string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}
	if args[0] != "set" {
		return a.failInvalid(extractRequestedFormat(args), "invalid_argument", fmt.Sprintf("未知 plugin subscription 子命令: %s", args[0]), "", help)
	}
	specs := []input.FlagSpec{
		{Name: "auto-update", Kind: input.FlagBool},
		{Name: "enabled", Kind: input.FlagBool},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}
	if len(parsed.Positionals) != 1 {
		return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin subscription set 需要 1 个 <plugin-id> 参数", "", "")
	}
	rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
	if pCode != protocol.ExitOK {
		return pCode
	}
	overrides := map[string]any{}
	if v, exists := parsed.Bool("auto-update"); exists {
		overrides["auto_update"] = v
	}
	if v, exists := parsed.Bool("enabled"); exists {
		overrides["enabled"] = v
	}
	payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
	if err != nil {
		return a.handleInputError(parsed.Global.Format, err)
	}
	if len(payload) == 0 {
		return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin subscription set 需要 --auto-update / --enabled 或 --json", "", "")
	}
	return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
		Resource: "plugin",
		Action:   "subscription-set",
		Target:   parsed.Positionals[0],
		Payload:  payload,
	})
}

func (a *App) runPluginAutomation(ctx context.Context, args []string, help string) int {
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
	case "list":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin automation list 需要 1 个 <plugin-id> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "plugin",
			Action:   "automation-list",
			Target:   parsed.Positionals[0],
		})
	case "create":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin automation create 需要 1 个 <plugin-id> 参数", "", "")
		}
		payload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		if !hasPayload {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin automation create 需要 --json、--file 或 --stdin 载荷", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "plugin",
			Action:   "automation-create",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	case "update":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin automation update 需要 <plugin-id> <automation-id>", "", "")
		}
		payload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		if !hasPayload {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin automation update 需要 --json、--file 或 --stdin 载荷", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "plugin",
			Action:    "automation-update",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
			Payload:   payload,
		})
	case "delete":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin automation delete 需要 <plugin-id> <automation-id>", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "plugin",
			Action:    "automation-delete",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 plugin automation 子命令: %s", sub), "", help)
	}
}

func (a *App) runPluginTrustedKeys(ctx context.Context, args []string, help string) int {
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
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin trusted-keys list 不接受位置参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "plugin", Action: "trusted-keys-list"})
	case "add", "create", "put", "delete", "remove":
		return a.failPolicy(
			extractRequestedFormat(args[1:]),
			"禁止通过 xlyra-cli 增删 JS 插件信任公钥（Tier 0）",
			"js-plugin-trusted-keys",
			"请在 Web 控制台安全设置中管理信任公钥",
		)
	default:
		return a.failInvalid(extractRequestedFormat(args[1:]), "invalid_argument", fmt.Sprintf("未知 plugin trusted-keys 子命令: %s", sub), "", help)
	}
}

func (a *App) runPluginMarket(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli plugin-market status|list|sync
  xlyra-cli plugin-market get <plugin-id>
  xlyra-cli plugin-market install <plugin-id> [--version V]`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "version", Kind: input.FlagString},
		{Name: "search", Kind: input.FlagString},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "status", "sync":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("plugin-market %s 不接受位置参数", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "plugin-market", Action: sub})
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin-market list 不接受位置参数", "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.String("search"); exists {
			params["search"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "plugin-market", Action: "list", Params: params})
	case "get":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin-market get 需要 1 个 <plugin-id> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "plugin-market", Action: "get", Target: parsed.Positionals[0]})
	case "install":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "plugin-market install 需要 1 个 <plugin-id> 参数", "", "")
		}
		payload := map[string]any{}
		if v, exists := parsed.String("version"); exists {
			payload["version"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "plugin-market",
			Action:   "install",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 plugin-market 子命令: %s", sub), "", help)
	}
}
