package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/Yachiiiiyo/xLyra-cli/internal/input"
	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
)

func (a *App) runAPIKey(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra api-key list
  xlyra api-key get|rotate|delete <id|name>
  xlyra api-key create --name N [--json '{...}']
  xlyra api-key update <id|name> (--json '{...}' | --file <path> | --stdin)
  xlyra api-key reorder <id1,id2,...>
  xlyra api-key set-sites <id|name> --policy allow_all|allow_list [--ids id1,id2]
  xlyra api-key set-site-groups <id|name> --ids id1,id2
  xlyra api-key set-site-models <id|name> --policy allow_all|allow_list [--ids id1,id2]
  xlyra api-key set-mappings <id|name> (--json '[...]' | --file <path> | --stdin)
  xlyra api-key quota increase <id|name> --amount F
  xlyra api-key quota reset <id|name>
  xlyra api-key check-model <id|name> <model_key>`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	if sub == "quota" {
		return a.runAPIKeyQuota(ctx, args[1:], help)
	}

	specs := []input.FlagSpec{
		{Name: "name", Kind: input.FlagString},
		{Name: "policy", Kind: input.FlagString},
		{Name: "ids", Kind: input.FlagCSV},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key list 不接受位置参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "api-key", Action: "list"})
	case "get", "rotate", "delete":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("api-key %s 需要 1 个 <id|name> 参数", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "api-key", Action: sub, Target: parsed.Positionals[0]})
	case "create":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key create 不接受位置参数", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		if v, exists := parsed.String("name"); exists {
			overrides["name"] = v
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if name, _ := payload["name"].(string); strings.TrimSpace(name) == "" {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key create 需要指定 --name", "name", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "api-key", Action: "create", Payload: payload})
	case "update":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key update 需要 1 个 <id|name> 参数", "", "")
		}
		payload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		if !hasPayload {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key update 需要通过 --json、--file 或 --stdin 提供更新载荷", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "api-key",
			Action:   "update",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	case "reorder":
		if len(parsed.Positionals) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key reorder 需要 <id1,id2,...>", "", "")
		}
		var ids []string
		for _, arg := range parsed.Positionals {
			ids = append(ids, input.SplitCSV(arg)...)
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "api-key",
			Action:   "reorder",
			Payload:  map[string]any{"ids": ids},
		})
	case "set-sites", "set-site-models":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("api-key %s 需要 1 个 <id|name> 参数", sub), "", "")
		}
		policy, hasPolicy := parsed.String("policy")
		if !hasPolicy || strings.TrimSpace(policy) == "" {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("api-key %s 需要指定 --policy allow_all|allow_list", sub), "policy", "")
		}
		payload := map[string]any{"policy": policy}
		if ids, hasIDs := parsed.Strings("ids"); hasIDs {
			payload["ids"] = ids
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "api-key",
			Action:   sub,
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	case "set-site-groups":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key set-site-groups 需要 1 个 <id|name> 参数", "", "")
		}
		ids, hasIDs := parsed.Strings("ids")
		if !hasIDs {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key set-site-groups 需要 --ids", "ids", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "api-key",
			Action:   "set-site-groups",
			Target:   parsed.Positionals[0],
			Payload:  map[string]any{"ids": ids},
		})
	case "set-mappings":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key set-mappings 需要 1 个 <id|name> 参数", "", "")
		}
		payload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		if !hasPayload {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key set-mappings 需要 --json、--file 或 --stdin 载荷", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "api-key",
			Action:   "set-mappings",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	case "check-model":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key check-model 需要 <id|name> <model_key>", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "api-key",
			Action:    "check-model",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 api-key 子命令: %s", sub), "", help)
	}
}

func (a *App) runAPIKeyQuota(ctx context.Context, args []string, help string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}
	sub := args[0]
	specs := []input.FlagSpec{{Name: "amount", Kind: input.FlagFloat}}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "increase":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key quota increase 需要 1 个 <id|name> 参数", "", "")
		}
		amount, hasAmount := parsed.Float("amount")
		if !hasAmount {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key quota increase 需要指定 --amount", "amount", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "api-key",
			Action:   "quota-increase",
			Target:   parsed.Positionals[0],
			Payload:  map[string]any{"amount": amount},
		})
	case "reset":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "api-key quota reset 需要 1 个 <id|name> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "api-key",
			Action:   "quota-reset",
			Target:   parsed.Positionals[0],
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 api-key quota 子命令: %s", sub), "", help)
	}
}

func (a *App) runRoute(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra route overview [--window-hours 24]
  xlyra route candidates <model_key> [--debug]
  xlyra route select --model <model_key> [--api-key ID] [--debug]
  xlyra route failover --model <model_key> --failed-site-ids id1,id2 [--debug]
  xlyra route traces [--model <model_key>] [--limit 20]`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "window-hours", Kind: input.FlagInt},
		{Name: "debug", Kind: input.FlagBool},
		{Name: "model", Kind: input.FlagString},
		{Name: "api-key", Kind: input.FlagString},
		{Name: "failed-site-ids", Kind: input.FlagCSV},
		{Name: "limit", Kind: input.FlagInt},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "overview":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "route overview 不接受位置参数", "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.Int("window-hours"); exists {
			params["window_hours"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "route", Action: "overview", Params: params})
	case "candidates":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "route candidates 需要 1 个 <model_key> 参数", "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.Bool("debug"); exists {
			params["debug"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "route",
			Action:   "candidates",
			Target:   parsed.Positionals[0],
			Params:   params,
		})
	case "select":
		model, hasModel := parsed.String("model")
		if !hasModel || strings.TrimSpace(model) == "" {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "route select 需要指定 --model", "model", "")
		}
		params := map[string]any{"model": model}
		if v, exists := parsed.String("api-key"); exists {
			params["api_key"] = v
		}
		if v, exists := parsed.Bool("debug"); exists {
			params["debug"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "route", Action: "select", Params: params})
	case "failover":
		model, hasModel := parsed.String("model")
		failedIDs, hasFailed := parsed.Strings("failed-site-ids")
		if !hasModel || !hasFailed || len(failedIDs) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "route failover 需要 --model 与 --failed-site-ids", "", "")
		}
		params := map[string]any{
			"model":           model,
			"failed_site_ids": failedIDs,
		}
		if v, exists := parsed.Bool("debug"); exists {
			params["debug"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "route", Action: "failover", Params: params})
	case "traces":
		params := map[string]any{}
		if v, exists := parsed.String("model"); exists {
			params["model"] = v
		}
		if v, exists := parsed.Int("limit"); exists {
			params["limit"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "route", Action: "traces", Params: params})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 route 子命令: %s", sub), "", help)
	}
}

func (a *App) runCooldown(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra cooldown list
  xlyra cooldown create --site <site> [--site-model ID] [--credential ID] [--scope S] [--source S] --duration 300 --reason "..."
  xlyra cooldown clear --site <site> [--site-model ID] [--credential ID] [--source S]`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "site", Kind: input.FlagString},
		{Name: "site-model", Kind: input.FlagString},
		{Name: "credential", Kind: input.FlagString},
		{Name: "scope", Kind: input.FlagString},
		{Name: "source", Kind: input.FlagString},
		{Name: "duration", Kind: input.FlagInt},
		{Name: "reason", Kind: input.FlagString},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "cooldown list 不接受位置参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "cooldown", Action: "list"})
	case "create":
		site, hasSite := parsed.String("site")
		dur, hasDur := parsed.Int("duration")
		if !hasSite || strings.TrimSpace(site) == "" || !hasDur || dur <= 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "cooldown create 需要指定 --site 与正整数 --duration", "", "")
		}
		payload := map[string]any{
			"site":             site,
			"duration_seconds": dur,
		}
		for flagName, jsonKey := range map[string]string{
			"site-model": "site_model_id",
			"credential": "site_credential_id",
			"scope":      "scope",
			"source":     "source",
			"reason":     "reason",
		} {
			if v, exists := parsed.String(flagName); exists {
				payload[jsonKey] = v
			}
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "cooldown", Action: "create", Payload: payload})
	case "clear":
		site, hasSite := parsed.String("site")
		if !hasSite || strings.TrimSpace(site) == "" {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "cooldown clear 需要指定 --site", "site", "")
		}
		payload := map[string]any{"site": site}
		for flagName, jsonKey := range map[string]string{
			"site-model": "site_model_id",
			"credential": "site_credential_id",
			"source":     "source",
		} {
			if v, exists := parsed.String(flagName); exists {
				payload[jsonKey] = v
			}
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "cooldown", Action: "clear", Payload: payload})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 cooldown 子命令: %s", sub), "", help)
	}
}
