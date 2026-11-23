package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/Yachiiiiyo/xLyra-cli/internal/input"
	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
)

func (a *App) runStatus(ctx context.Context, args []string) int {
	parsed, ok, code := a.parse(args, nil, "用法: xlyra status")
	if !ok {
		return code
	}
	if len(parsed.Positionals) > 0 {
		return a.failInvalid(parsed.Global.Format, "invalid_argument", "status 不接受额外位置参数", "", "")
	}
	return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
		Resource: "status",
		Action:   "get",
	})
}

func (a *App) runSchema(ctx context.Context, args []string) int {
	parsed, ok, code := a.parse(args, nil, "用法: xlyra schema [domain]")
	if !ok {
		return code
	}
	if len(parsed.Positionals) > 1 {
		return a.failInvalid(parsed.Global.Format, "invalid_argument", "schema 最多接受 1 个 <domain> 参数", "", "")
	}
	req := protocol.CallRequest{Resource: "schema", Action: "list"}
	if len(parsed.Positionals) == 1 {
		req.Action = "get"
		req.Target = parsed.Positionals[0]
	}
	return a.dispatch(ctx, parsed.Global, req)
}

func (a *App) runConfig(ctx context.Context, args []string) int {
	const help = `用法:
  xlyra config list
  xlyra config describe <dot.path>
  xlyra config get <dot.path>
  xlyra config set <dot.path> <val>
  xlyra config set <k1=v1> [k2=v2 ...]
  xlyra config patch <domain> (--json '{...}' | --file <path> | --stdin)
  xlyra config reset <dot.path>`

	if len(args) == 0 {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	sub := args[0]
	if sub == "-h" || sub == "--help" {
		_, _ = fmt.Fprintln(a.Stdout, help)
		return protocol.ExitOK
	}

	parsed, ok, code := a.parse(args[1:], nil, help)
	if !ok {
		return code
	}

	switch sub {
	case "list":
		if len(parsed.Positionals) != 0 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "config list 不接受位置参数", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{Resource: "config", Action: "list"})
	case "describe", "get", "reset":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("config %s 需要 1 个 <dot.path> 参数", sub), "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "config",
			Action:   sub,
			Target:   parsed.Positionals[0],
		})
	case "set":
		return a.runConfigSet(ctx, parsed)
	case "patch":
		if len(parsed.Positionals) != 1 {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "config patch 需要 1 个 <domain> 参数", "", "")
		}
		payload, hasPayload, pCode := a.loadPayload(parsed.Global)
		if pCode != protocol.ExitOK {
			return pCode
		}
		if !hasPayload {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "config patch 需要通过 --json、--file 或 --stdin 提供对象载荷", "", "")
		}
		if _, isObj := payload.(map[string]any); !isObj {
			return a.failInvalid(parsed.Global.Format, "invalid_argument", "config patch 的载荷必须是 JSON 对象", "", "")
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "config",
			Action:   "patch",
			Target:   parsed.Positionals[0],
			Payload:  payload,
		})
	default:
		return a.failInvalid(parsed.Global.Format, "invalid_argument", fmt.Sprintf("未知 config 子命令: %s", sub), "", help)
	}
}

func (a *App) runConfigSet(ctx context.Context, parsed input.ParsedArgs) int {
	pos := parsed.Positionals
	if len(pos) == 0 {
		return a.failInvalid(parsed.Global.Format, "invalid_argument", "config set 需要 <dot.path> <val> 或 k=v 参数", "", "")
	}

	allKV := true
	for _, item := range pos {
		if !strings.Contains(item, "=") {
			allKV = false
			break
		}
	}

	if allKV {
		kv, err := input.ParseKeyValues(pos)
		if err != nil {
			return a.handleInputError(parsed.Global.Format, err)
		}
		target := ""
		if len(pos) == 1 {
			for k := range kv {
				target = k
			}
		}
		return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
			Resource: "config",
			Action:   "set",
			Target:   target,
			Payload:  kv,
		})
	}

	if len(pos) != 2 || strings.Contains(pos[0], "=") {
		return a.failInvalid(parsed.Global.Format, "invalid_argument", "config set 用法为: config set <dot.path> <val> 或多个 k=v", "", "")
	}
	typedVal, err := input.ParseTypedValue(pos[1])
	if err != nil {
		return a.handleInputError(parsed.Global.Format, err)
	}
	return a.dispatch(ctx, parsed.Global, protocol.CallRequest{
		Resource: "config",
		Action:   "set",
		Target:   pos[0],
		Payload:  map[string]any{pos[0]: typedVal},
	})
}
