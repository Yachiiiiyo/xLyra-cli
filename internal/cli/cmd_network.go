package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/Yachiiiiyo/xLyra-cli/internal/input"
	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
)

func mergePayloadMap(base any, hasBase bool, overrides map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(overrides))
	if hasBase {
		m, ok := base.(map[string]any)
		if !ok {
			return nil, &input.ValidationError{
				Code:    "invalid_argument",
				Message: "JSON 载荷必须是对象",
			}
		}
		for k, v := range m {
			out[k] = v
		}
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out, nil
}

func validateProxyPayloadURL(payload map[string]any) error {
	if rawURL, ok := payload["url"].(string); ok {
		return input.ValidateProxyURLNoPassword(rawURL)
	}
	return nil
}

func (a *App) runProxy(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli proxy list
  xlyra-cli proxy get <id|name>
  xlyra-cli proxy create --name N --type http|https|socks5 --url U
  xlyra-cli proxy update <id|name> [--name N] [--type T] [--url U]
  xlyra-cli proxy delete <id|name>
  xlyra-cli proxy test (<id|name> | --type T --url U)`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "name", Kind: input.FlagString},
		{Name: "type", Kind: input.FlagString},
		{Name: "url", Kind: input.FlagString},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "proxy list 不接受位置参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "proxy", Action: "list"})
	case "get", "delete":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("proxy %s 需要 1 个 <id|name> 参数", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "proxy", Action: sub, Target: parsed.Positionals[0]})
	case "create":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "proxy create 不接受位置参数", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		for _, k := range []string{"name", "type", "url"} {
			if v, exists := parsed.String(k); exists {
				overrides[k] = v
			}
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if err := validateProxyPayloadURL(payload); err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		for _, reqKey := range []string{"name", "type", "url"} {
			if s, _ := payload[reqKey].(string); strings.TrimSpace(s) == "" {
				return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("proxy create 缺少必要字段 %s", reqKey), reqKey, help)
			}
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "proxy", Action: "create", Payload: payload})
	case "update":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "proxy update 需要 1 个 <id|name> 参数", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		for _, k := range []string{"name", "type", "url"} {
			if v, exists := parsed.String(k); exists {
				overrides[k] = v
			}
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if len(payload) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "proxy update 至少需要提供一个待更新字段", "", "")
		}
		if err := validateProxyPayloadURL(payload); err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "proxy",
			Action:   "update",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	case "test":
		if len(parsed.Positionals) > 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "proxy test 最多接受 1 个 <id|name> 参数", "", "")
		}
		req := protocol.CallRequest{Resource: "proxy", Action: "test"}
		if len(parsed.Positionals) == 1 {
			req.Target = parsed.Positionals[0]
			return a.dispatch(ctx, parsed.Global, req)
		}
		overrides := map[string]any{}
		for _, k := range []string{"type", "url"} {
			if v, exists := parsed.String(k); exists {
				overrides[k] = v
			}
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if err := validateProxyPayloadURL(payload); err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if len(payload) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "proxy test 需要指定 <id|name> 或 --type 与 --url", "", help)
		}
		req.Payload = payload
		return a.dispatch(ctx, parsed.Global, req)
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 proxy 子命令: %s", sub), "", help)
	}
}

func (a *App) runSite(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli site list [--enabled true|false] [--oauth all|only|exclude] [--type T]
  xlyra-cli site get <id|slug>
  xlyra-cli site create (--json '{...}' | --file <path> | --stdin)
  xlyra-cli site update <id|slug> (--json '{...}' | --file <path> | --stdin)
  xlyra-cli site enable|disable|delete|sync-models <id|slug>
  xlyra-cli site set-gateway <id|slug> <k=v ...>
  xlyra-cli site set-plugin-order <id|slug> <p1,p2,...>
  xlyra-cli site refresh [<id|slug> | --all]
  xlyra-cli site health <id|slug> [--history] [--check]`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "enabled", Kind: input.FlagBool},
		{Name: "oauth", Kind: input.FlagString},
		{Name: "type", Kind: input.FlagString},
		{Name: "all", Kind: input.FlagBool},
		{Name: "history", Kind: input.FlagBool},
		{Name: "check", Kind: input.FlagBool},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site list 不接受位置参数", "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.Bool("enabled"); exists {
			params["enabled"] = v
		}
		if v, exists := parsed.String("oauth"); exists {
			params["oauth"] = v
		}
		if v, exists := parsed.String("type"); exists {
			params["type"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "site", Action: "list", Params: params})
	case "get", "enable", "disable", "delete", "sync-models":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("site %s 需要 1 个 <id|slug> 参数", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "site", Action: sub, Target: parsed.Positionals[0]})
	case "create":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site create 不接受位置参数", "", "")
		}
		payload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		if !hasPayload {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site create 需要通过 --json、--file 或 --stdin 提供站点配置对象", "", "")
		}
		if err := input.ValidateNoSensitiveRequestHeaders(payload); err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "site", Action: "create", Payload: payload})
	case "update":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site update 需要 1 个 <id|slug> 参数", "", "")
		}
		payload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		if !hasPayload {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site update 需要通过 --json、--file 或 --stdin 提供更新载荷", "", "")
		}
		if err := input.ValidateNoSensitiveRequestHeaders(payload); err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "site",
			Action:   "update",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	case "set-gateway":
		if len(parsed.Positionals) < 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site set-gateway 需要 <id|slug> 参数", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		var kv map[string]any
		if len(parsed.Positionals) > 1 {
			var err error
			kv, err = input.ParseKeyValues(parsed.Positionals[1:])
			if err != nil {
				return a.handleInputError(parsed.Global.Format, err)
			}
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, kv)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if len(payload) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site set-gateway 需要至少一个 gateway 字段或 --json 载荷", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "site",
			Action:   "set-gateway",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	case "set-plugin-order":
		if len(parsed.Positionals) < 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site set-plugin-order 需要 <id|slug> 与 <p1,p2,...>", "", "")
		}
		var order []string
		for _, arg := range parsed.Positionals[1:] {
			order = append(order, input.SplitCSV(arg)...)
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "site",
			Action:   "set-plugin-order",
			Target:   parsed.Positionals[0],
			Payload:  map[string]any{"plugin_ids": order},
		})
	case "refresh":
		all, _ := parsed.Bool("all")
		if all && len(parsed.Positionals) > 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site refresh 不能同时指定 <id|slug> 与 --all", "", "")
		}
		if !all && len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site refresh 需要指定 <id|slug> 或 --all", "", "")
		}
		req := protocol.CallRequest{Resource: "site", Action: "refresh"}
		if all {
			req.Params = map[string]any{"all": true}
		} else {
			req.Target = parsed.Positionals[0]
		}
		return a.dispatch(ctx, parsed.Global, req)
	case "health":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site health 需要 1 个 <id|slug> 参数", "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.Bool("history"); exists {
			params["history"] = v
		}
		if v, exists := parsed.Bool("check"); exists {
			params["check"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "site",
			Action:   "health",
			Target:   parsed.Positionals[0],
			Params:   params,
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 site 子命令: %s", sub), "", help)
	}
}

func (a *App) runSiteGroup(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli site-group list
  xlyra-cli site-group get|delete <id|name>
  xlyra-cli site-group create [--name N] [--description D] [--site-ids id1,id2] [--json '{...}']
  xlyra-cli site-group update <id|name> [--name N] [--description D] [--site-ids id1,id2] [--json '{...}']`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "name", Kind: input.FlagString},
		{Name: "description", Kind: input.FlagString},
		{Name: "site-ids", Kind: input.FlagCSV},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-group list 不接受位置参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "site-group", Action: "list"})
	case "get", "delete":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("site-group %s 需要 1 个 <id|name> 参数", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "site-group", Action: sub, Target: parsed.Positionals[0]})
	case "create", "update":
		if sub == "create" && len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-group create 不接受位置参数", "", "")
		}
		if sub == "update" && len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-group update 需要 1 个 <id|name> 参数", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		if v, exists := parsed.String("name"); exists {
			overrides["name"] = v
		}
		if v, exists := parsed.String("description"); exists {
			overrides["description"] = v
		}
		if v, exists := parsed.Strings("site-ids"); exists {
			overrides["site_ids"] = v
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if len(payload) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("site-group %s 需要参数或 --json 载荷", sub), "", help)
		}
		req := protocol.CallRequest{Resource: "site-group", Action: sub, Payload: payload}
		if sub == "update" {
			req.Target = parsed.Positionals[0]
		}
		return a.dispatch(ctx, parsed.Global, req)
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 site-group 子命令: %s", sub), "", help)
	}
}

func (a *App) runSiteKey(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli site-key list <site>
  xlyra-cli site-key update <site> <key-id> [--enabled B] [--name N] [--priority P] [--cost-multiplier M] [--cache-domain D]
  xlyra-cli site-key set-model <site> <key-id> --model M [--enabled B] [--endpoint-mode ...] [--endpoint-types ...]
  xlyra-cli site-key delete <site> <key-id>`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	if sub == "create" {
		return a.failPolicy(
			extractRequestedFormat(args[1:]),
			"一期禁止通过 xlyra-cli 创建站点上游 API Key（Tier 0）",
			"site-key",
			"请在 Web 控制台站点设置中添加上游 Key",
		)
	}

	specs := []input.FlagSpec{
		{Name: "enabled", Kind: input.FlagBool},
		{Name: "name", Kind: input.FlagString},
		{Name: "priority", Kind: input.FlagInt},
		{Name: "cost-multiplier", Kind: input.FlagFloat},
		{Name: "cache-domain", Kind: input.FlagString},
		{Name: "model", Kind: input.FlagString},
		{Name: "endpoint-mode", Kind: input.FlagString},
		{Name: "endpoint-types", Kind: input.FlagCSV},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-key list 需要 1 个 <site> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "site-key",
			Action:   "list",
			Target:   parsed.Positionals[0],
		})
	case "delete":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-key delete 需要 <site> <key-id>", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "site-key",
			Action:    "delete",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
		})
	case "update":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-key update 需要 <site> <key-id>", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		if v, exists := parsed.Bool("enabled"); exists {
			overrides["enabled"] = v
		}
		if v, exists := parsed.String("name"); exists {
			overrides["name"] = v
		}
		if v, exists := parsed.Int("priority"); exists {
			overrides["routing_priority"] = v
		}
		if v, exists := parsed.Float("cost-multiplier"); exists {
			overrides["upstream_cost_multiplier"] = v
		}
		if v, exists := parsed.String("cache-domain"); exists {
			overrides["cache_domain"] = v
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if len(payload) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-key update 至少需要一个更新字段", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "site-key",
			Action:    "update",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
			Payload:   payload,
		})
	case "set-model":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-key set-model 需要 <site> <key-id>", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		if v, exists := parsed.String("model"); exists {
			overrides["model"] = v
		}
		if v, exists := parsed.Bool("enabled"); exists {
			overrides["enabled"] = v
		}
		if v, exists := parsed.String("endpoint-mode"); exists {
			overrides["endpoint_mode"] = v
		}
		if v, exists := parsed.Strings("endpoint-types"); exists {
			overrides["endpoint_types"] = v
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if m, _ := payload["model"].(string); strings.TrimSpace(m) == "" {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-key set-model 需要指定 --model", "model", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "site-key",
			Action:    "set-model",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
			Payload:   payload,
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 site-key 子命令: %s", sub), "", help)
	}
}

func (a *App) runSiteAccount(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli site-account list <site>
  xlyra-cli site-account update <site> <cred-id> --enabled true|false
  xlyra-cli site-account set-model <site> <cred-id> --model M [--enabled B] [--endpoint-mode ...] [--endpoint-types ...]
  xlyra-cli site-account refresh <site> [<cred-id> | --all]`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "enabled", Kind: input.FlagBool},
		{Name: "model", Kind: input.FlagString},
		{Name: "endpoint-mode", Kind: input.FlagString},
		{Name: "endpoint-types", Kind: input.FlagCSV},
		{Name: "all", Kind: input.FlagBool},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-account list 需要 1 个 <site> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "site-account",
			Action:   "list",
			Target:   parsed.Positionals[0],
		})
	case "update":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-account update 需要 <site> <cred-id>", "", "")
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
		if len(payload) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-account update 需要 --enabled 或 --json", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "site-account",
			Action:    "update",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
			Payload:   payload,
		})
	case "set-model":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-account set-model 需要 <site> <cred-id>", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		if v, exists := parsed.String("model"); exists {
			overrides["model"] = v
		}
		if v, exists := parsed.Bool("enabled"); exists {
			overrides["enabled"] = v
		}
		if v, exists := parsed.String("endpoint-mode"); exists {
			overrides["endpoint_mode"] = v
		}
		if v, exists := parsed.Strings("endpoint-types"); exists {
			overrides["endpoint_types"] = v
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if len(payload) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-account set-model 需要参数或 --json", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "site-account",
			Action:    "set-model",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
			Payload:   payload,
		})
	case "refresh":
		all, _ := parsed.Bool("all")
		if len(parsed.Positionals) < 1 || len(parsed.Positionals) > 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-account refresh 需要 <site> [<cred-id> | --all]", "", "")
		}
		req := protocol.CallRequest{
			Resource: "site-account",
			Action:   "refresh",
			Target:   parsed.Positionals[0],
		}
		if len(parsed.Positionals) == 2 {
			if all {
				return a.failInvalid(parsed.Global.Format, "invalid_argument", "不能同时指定 <cred-id> 与 --all", "", "")
			}
			req.SubTarget = parsed.Positionals[1]
		} else {
			if !all {
				return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-account refresh 需要指定 <cred-id> 或 --all", "", "")
			}
			req.Params = map[string]any{"all": true}
		}
		return a.dispatch(ctx, parsed.Global, req)
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 site-account 子命令: %s", sub), "", help)
	}
}

func (a *App) runOAuth(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra-cli oauth list
  xlyra-cli oauth get|refresh <conn-id>
  xlyra-cli oauth set-model <conn-id> --site-model-id ID --enabled true|false`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "site-model-id", Kind: input.FlagString},
		{Name: "enabled", Kind: input.FlagBool},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "oauth list 不接受位置参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "oauth", Action: "list"})
	case "get", "refresh":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("oauth %s 需要 1 个 <conn-id> 参数", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "oauth", Action: sub, Target: parsed.Positionals[0]})
	case "set-model":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "oauth set-model 需要 1 个 <conn-id> 参数", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		if v, exists := parsed.String("site-model-id"); exists {
			overrides["site_model_id"] = v
		}
		if v, exists := parsed.Bool("enabled"); exists {
			overrides["enabled"] = v
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if id, _ := payload["site_model_id"].(string); strings.TrimSpace(id) == "" {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "oauth set-model 需要指定 --site-model-id", "site-model-id", "")
		}
		if _, hasEnabled := payload["enabled"]; !hasEnabled {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "oauth set-model 需要指定 --enabled true|false", "enabled", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "oauth",
			Action:   "set-model",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 oauth 子命令: %s", sub), "", help)
	}
}
