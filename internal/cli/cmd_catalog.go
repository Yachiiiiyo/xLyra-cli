package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/Yachiiiiyo/xLyra-cli/internal/input"
	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
)

func (a *App) runModel(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra model list [--provider P] [--category C] [--search Q]
  xlyra model get <id|model_key> [--matrix]
  xlyra model create (--json '{...}' | --model-key K --display-name N --provider P ...)
  xlyra model update <id|model_key> (--json '{...}' | flags...)
  xlyra model delete <id|model_key>
  xlyra model alias add <model> <alias>
  xlyra model alias delete <model> <alias-id>
  xlyra model pricing set <model> [--input F] [--output F] [--cache-read-ratio F] [--cache-write-ratio F] [--cache-write-1h-ratio F]
  xlyra model pricing reset <model>`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	switch sub {
	case "alias":
		return a.runModelAlias(ctx, args[1:], help)
	case "pricing":
		return a.runModelPricing(ctx, args[1:], help)
	}

	specs := []input.FlagSpec{
		{Name: "provider", Kind: input.FlagString},
		{Name: "category", Kind: input.FlagString},
		{Name: "search", Kind: input.FlagString},
		{Name: "matrix", Kind: input.FlagBool},
		{Name: "model-key", Kind: input.FlagString},
		{Name: "display-name", Kind: input.FlagString},
		{Name: "status", Kind: input.FlagString},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "model list 不接受位置参数", "", "")
		}
		params := map[string]any{}
		for _, k := range []string{"provider", "category", "search"} {
			if v, exists := parsed.String(k); exists {
				params[k] = v
			}
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "model", Action: "list", Params: params})
	case "get":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "model get 需要 1 个 <id|model_key> 参数", "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.Bool("matrix"); exists {
			params["matrix"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "model",
			Action:   "get",
			Target:   parsed.Positionals[0],
			Params:   params,
		})
	case "delete":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "model delete 需要 1 个 <id|model_key> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "model", Action: "delete", Target: parsed.Positionals[0]})
	case "create", "update":
		if sub == "create" && len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "model create 不接受位置参数", "", "")
		}
		if sub == "update" && len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "model update 需要 1 个 <id|model_key> 参数", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		if v, exists := parsed.String("model-key"); exists {
			overrides["model_key"] = v
		}
		if v, exists := parsed.String("display-name"); exists {
			overrides["display_name"] = v
		}
		for _, k := range []string{"provider", "category", "status"} {
			if v, exists := parsed.String(k); exists {
				overrides[k] = v
			}
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if len(payload) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("model %s 需要参数或 --json 载荷", sub), "", "")
		}
		req := protocol.CallRequest{Resource: "model", Action: sub, Payload: payload}
		if sub == "update" {
			req.Target = parsed.Positionals[0]
		}
		return a.dispatch(ctx, parsed.Global, req)
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 model 子命令: %s", sub), "", help)
	}
}

func (a *App) runModelAlias(ctx context.Context, args []string, help string) int {
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
	case "add":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "model alias add 需要 <model> <alias>", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "model",
			Action:   "alias-add",
			Target:   parsed.Positionals[0],
			Payload:  map[string]any{"alias": parsed.Positionals[1]},
		})
	case "delete":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "model alias delete 需要 <model> <alias-id>", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "model",
			Action:    "alias-delete",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 model alias 子命令: %s", sub), "", help)
	}
}

func (a *App) runModelPricing(ctx context.Context, args []string, help string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}
	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "input", Kind: input.FlagFloat},
		{Name: "output", Kind: input.FlagFloat},
		{Name: "cache-read-ratio", Kind: input.FlagFloat},
		{Name: "cache-write-ratio", Kind: input.FlagFloat},
		{Name: "cache-write-1h-ratio", Kind: input.FlagFloat},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "set":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "model pricing set 需要 1 个 <model> 参数", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		mappings := map[string]string{
			"input":                "input_price",
			"output":               "output_price",
			"cache-read-ratio":     "cache_read_ratio",
			"cache-write-ratio":    "cache_write_ratio",
			"cache-write-1h-ratio": "cache_write_1h_ratio",
		}
		for flagName, jsonKey := range mappings {
			if v, exists := parsed.Float(flagName); exists {
				overrides[jsonKey] = v
			}
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if len(payload) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "model pricing set 需要指定至少一项定价字段或 --json", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "model",
			Action:   "pricing-set",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	case "reset":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "model pricing reset 需要 1 个 <model> 参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "model",
			Action:   "pricing-reset",
			Target:   parsed.Positionals[0],
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 model pricing 子命令: %s", sub), "", help)
	}
}

func (a *App) runSiteModel(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra site-model list [--site S] [--canonical M] [--enabled true|false]
  xlyra site-model create <site> --upstream-name N [--canonical M] [--endpoints ...] [--credential-ids ...] [--enabled B]
  xlyra site-model update <site> <model-id> [--enabled true|false] [--json '{...}']
  xlyra site-model batch-status <site> --ids id1,id2 --enabled true|false
  xlyra site-model bind-canonical <site-model-id> <canonical-model-id|none>
  xlyra site-model test <site> <model-id> [--protocol P] [--prompt ...]
  xlyra site-model delete <site> <model-id>`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{
		{Name: "site", Kind: input.FlagString},
		{Name: "canonical", Kind: input.FlagString},
		{Name: "enabled", Kind: input.FlagBool},
		{Name: "upstream-name", Kind: input.FlagString},
		{Name: "endpoints", Kind: input.FlagCSV},
		{Name: "credential-ids", Kind: input.FlagCSV},
		{Name: "ids", Kind: input.FlagCSV},
		{Name: "protocol", Kind: input.FlagString},
		{Name: "prompt", Kind: input.FlagString},
	}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-model list 不接受位置参数", "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.String("site"); exists {
			params["site"] = v
		}
		if v, exists := parsed.String("canonical"); exists {
			params["canonical"] = v
		}
		if v, exists := parsed.Bool("enabled"); exists {
			params["enabled"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "site-model", Action: "list", Params: params})
	case "create":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-model create 需要 1 个 <site> 参数", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		if v, exists := parsed.String("upstream-name"); exists {
			overrides["upstream_model_name"] = v
		}
		if v, exists := parsed.String("canonical"); exists {
			overrides["canonical_model_id"] = v
		}
		if v, exists := parsed.Strings("endpoints"); exists {
			overrides["supported_endpoint_types"] = v
		}
		if v, exists := parsed.Strings("credential-ids"); exists {
			overrides["site_credential_ids"] = v
		}
		if v, exists := parsed.Bool("enabled"); exists {
			overrides["enabled"] = v
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if name, _ := payload["upstream_model_name"].(string); strings.TrimSpace(name) == "" {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-model create 需要指定 --upstream-name", "upstream_model_name", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "site-model",
			Action:   "create",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	case "update":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-model update 需要 <site> <model-id>", "", "")
		}
		rawPayload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		overrides := map[string]any{}
		if v, exists := parsed.Bool("enabled"); exists {
			overrides["enabled"] = v
		}
		if v, exists := parsed.Strings("endpoints"); exists {
			overrides["supported_endpoint_types"] = v
		}
		if v, exists := parsed.Strings("credential-ids"); exists {
			overrides["site_credential_ids"] = v
		}
		payload, err := mergePayloadMap(rawPayload, hasPayload, overrides)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		if len(payload) == 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-model update 需要参数或 --json", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "site-model",
			Action:    "update",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
			Payload:   payload,
		})
	case "batch-status":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-model batch-status 需要 1 个 <site> 参数", "", "")
		}
		ids, okIDs := parsed.Strings("ids")
		enabled, okEnabled := parsed.Bool("enabled")
		if !okIDs || len(ids) == 0 || !okEnabled {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-model batch-status 需要 --ids 与 --enabled", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "site-model",
			Action:   "batch-status",
			Target:   parsed.Positionals[0],
			Payload:  map[string]any{"ids": ids, "enabled": enabled},
		})
	case "bind-canonical":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-model bind-canonical 需要 <site-model-id> <canonical-model-id|none>", "", "")
		}
		var canonVal any = parsed.Positionals[1]
		if strings.EqualFold(parsed.Positionals[1], "none") {
			canonVal = nil
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "site-model",
			Action:   "bind-canonical",
			Target:   parsed.Positionals[0],
			Payload:  map[string]any{"canonical_model_id": canonVal},
		})
	case "test":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-model test 需要 <site> <model-id>", "", "")
		}
		payload := map[string]any{}
		if v, exists := parsed.String("protocol"); exists {
			payload["protocol"] = v
		}
		if v, exists := parsed.String("prompt"); exists {
			payload["prompt"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "site-model",
			Action:    "test",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
			Payload:   payload,
		})
	case "delete":
		if len(parsed.Positionals) != 2 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "site-model delete 需要 <site> <model-id>", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource:  "site-model",
			Action:    "delete",
			Target:    parsed.Positionals[0],
			SubTarget: parsed.Positionals[1],
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 site-model 子命令: %s", sub), "", help)
	}
}

func (a *App) runPricing(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra pricing list [--site S]
  xlyra pricing set <site-model-id> (--json '{...}' | --file <path> | --stdin)
  xlyra pricing bulk-set (--json '{...}' | --file <path> | --stdin)
  xlyra pricing reset (<site-model-id> | --site <site>)`

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	specs := []input.FlagSpec{{Name: "site", Kind: input.FlagString}}
	parsed, ok, code := a.parse(args[1:], specs, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "pricing list 不接受位置参数", "", "")
		}
		params := map[string]any{}
		if v, exists := parsed.String("site"); exists {
			params["site"] = v
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "pricing", Action: "list", Params: params})
	case "set":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "pricing set 需要 1 个 <site-model-id> 参数", "", "")
		}
		payload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		if !hasPayload {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "pricing set 需要 --json、--file 或 --stdin 载荷", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "pricing",
			Action:   "set",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	case "bulk-set":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "pricing bulk-set 不接受位置参数", "", "")
		}
		payload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		if !hasPayload {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "pricing bulk-set 需要 --json、--file 或 --stdin 载荷", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "pricing",
			Action:   "bulk-set",
			Payload:  payload,
		})
	case "reset":
		siteFlag, hasSite := parsed.String("site")
		if hasSite && len(parsed.Positionals) > 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "pricing reset 不能同时指定 <site-model-id> 与 --site", "", "")
		}
		if !hasSite && len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "pricing reset 需要指定 <site-model-id> 或 --site <site>", "", "")
		}
		req := protocol.CallRequest{Resource: "pricing", Action: "reset"}
		if hasSite {
			req.Params = map[string]any{"site": siteFlag}
		} else {
			req.Target = parsed.Positionals[0]
		}
		return a.dispatch(ctx, parsed.Global, req)
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 pricing 子命令: %s", sub), "", help)
	}
}
