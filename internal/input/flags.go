// Package input provides flexible CLI flag parsing, bounded JSON payload loading,
// and client-side secret/path boundary checks for xlyra-cli.
package input

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// FlagKind defines how a flag's value is parsed from argv tokens.
type FlagKind int

const (
	FlagString FlagKind = iota
	FlagBool
	FlagInt
	FlagFloat
	FlagCSV
)

// FlagSpec describes a single command-specific CLI flag.
type FlagSpec struct {
	Name string
	Kind FlagKind
}

// GlobalFlags captures the cross-cutting flags supported across all xlyra-cli commands.
type GlobalFlags struct {
	Format  string
	Fields  []string
	DryRun  bool
	JSON    string
	File    string
	Stdin   bool
	IfMatch string
	Help    bool
}

// ParsedArgs contains global flags, command-specific flag values, and positional arguments.
type ParsedArgs struct {
	Global      GlobalFlags
	Flags       map[string]any
	Positionals []string
}

// ValidationError marks an input/flag parsing error that maps to exit code 2 (or 4 for policy blocks).
type ValidationError struct {
	Code    string
	Message string
	Field   string
	Hint    string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// Has returns true if the named command flag was explicitly supplied.
func (p ParsedArgs) Has(name string) bool {
	_, ok := p.Flags[name]
	return ok
}

// String returns a string flag value if supplied.
func (p ParsedArgs) String(name string) (string, bool) {
	v, ok := p.Flags[name]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// Bool returns a boolean flag value if supplied.
func (p ParsedArgs) Bool(name string) (bool, bool) {
	v, ok := p.Flags[name]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// Int returns an int64 flag value if supplied.
func (p ParsedArgs) Int(name string) (int64, bool) {
	v, ok := p.Flags[name]
	if !ok {
		return 0, false
	}
	n, ok := v.(int64)
	return n, ok
}

// Float returns a float64 flag value if supplied.
func (p ParsedArgs) Float(name string) (float64, bool) {
	v, ok := p.Flags[name]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

// Strings returns a CSV flag slice if supplied.
func (p ParsedArgs) Strings(name string) ([]string, bool) {
	v, ok := p.Flags[name]
	if !ok {
		return nil, false
	}
	s, ok := v.([]string)
	return s, ok
}

// ParseArgs parses global flags, command flags defined in specs, and positional arguments
// in any order while preserving tokens after "--" as positionals.
func ParseArgs(args []string, specs []FlagSpec) (ParsedArgs, error) {
	specMap := make(map[string]FlagSpec, len(specs))
	for _, s := range specs {
		specMap[s.Name] = s
	}

	out := ParsedArgs{
		Global: GlobalFlags{Format: "json"},
		Flags:  map[string]any{},
	}

	endOfFlags := false
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if endOfFlags {
			out.Positionals = append(out.Positionals, tok)
			continue
		}
		if tok == "--" {
			endOfFlags = true
			continue
		}
		if tok == "-h" || tok == "--help" {
			out.Global.Help = true
			continue
		}
		if !strings.HasPrefix(tok, "-") || tok == "-" || isNegativeNumber(tok) {
			out.Positionals = append(out.Positionals, tok)
			continue
		}
		if !strings.HasPrefix(tok, "--") {
			return out, &ValidationError{
				Code:    "invalid_argument",
				Message: fmt.Sprintf("不支持的短参数: %s", tok),
				Hint:    "请使用完整参数名（例如 --help、--format、--json）",
			}
		}

		namePart := tok[2:]
		name, eqVal, hasEq := strings.Cut(namePart, "=")
		if name == "" {
			return out, &ValidationError{Code: "invalid_argument", Message: "空参数名 '--'"}
		}

		consumed, handled, err := parseGlobalFlag(name, eqVal, hasEq, args, i, &out.Global)
		if err != nil {
			return out, err
		}
		if handled {
			i += consumed
			continue
		}

		spec, ok := specMap[name]
		if !ok {
			return out, &ValidationError{
				Code:    "invalid_argument",
				Message: fmt.Sprintf("未知参数: --%s", name),
				Hint:    "请运行子命令的 --help 查看支持的参数",
			}
		}

		val, step, err := parseFlagValue(spec.Name, spec.Kind, eqVal, hasEq, args, i)
		if err != nil {
			return out, err
		}
		out.Flags[spec.Name] = val
		i += step
	}

	switch out.Global.Format {
	case "json", "table", "yaml":
	default:
		return out, &ValidationError{
			Code:    "invalid_argument",
			Message: fmt.Sprintf("不支持的输出格式 --format %q", out.Global.Format),
			Field:   "format",
			Hint:    "可选值: json, table, yaml",
		}
	}

	return out, nil
}

func parseGlobalFlag(name, eqVal string, hasEq bool, args []string, idx int, g *GlobalFlags) (int, bool, error) {
	switch name {
	case "format":
		val, step, err := parseFlagValue(name, FlagString, eqVal, hasEq, args, idx)
		if err != nil {
			return 0, true, err
		}
		g.Format = strings.ToLower(strings.TrimSpace(val.(string)))
		return step, true, nil
	case "fields":
		val, step, err := parseFlagValue(name, FlagCSV, eqVal, hasEq, args, idx)
		if err != nil {
			return 0, true, err
		}
		g.Fields = val.([]string)
		return step, true, nil
	case "dry-run":
		val, step, err := parseFlagValue(name, FlagBool, eqVal, hasEq, args, idx)
		if err != nil {
			return 0, true, err
		}
		g.DryRun = val.(bool)
		return step, true, nil
	case "json":
		val, step, err := parseFlagValue(name, FlagString, eqVal, hasEq, args, idx)
		if err != nil {
			return 0, true, err
		}
		g.JSON = val.(string)
		return step, true, nil
	case "file":
		val, step, err := parseFlagValue(name, FlagString, eqVal, hasEq, args, idx)
		if err != nil {
			return 0, true, err
		}
		g.File = val.(string)
		return step, true, nil
	case "stdin":
		val, step, err := parseFlagValue(name, FlagBool, eqVal, hasEq, args, idx)
		if err != nil {
			return 0, true, err
		}
		g.Stdin = val.(bool)
		return step, true, nil
	case "if-match":
		val, step, err := parseFlagValue(name, FlagString, eqVal, hasEq, args, idx)
		if err != nil {
			return 0, true, err
		}
		g.IfMatch = val.(string)
		return step, true, nil
	default:
		return 0, false, nil
	}
}

func parseFlagValue(name string, kind FlagKind, eqVal string, hasEq bool, args []string, idx int) (any, int, error) {
	if kind == FlagBool {
		if hasEq {
			b, err := parseStrictBool(eqVal)
			if err != nil {
				return nil, 0, &ValidationError{
					Code:    "invalid_argument",
					Message: fmt.Sprintf("参数 --%s 需要布尔值 (true|false)，收到 %q", name, eqVal),
					Field:   name,
				}
			}
			return b, 0, nil
		}
		if idx+1 < len(args) {
			if b, err := parseStrictBool(args[idx+1]); err == nil {
				return b, 1, nil
			}
		}
		return true, 0, nil
	}

	var raw string
	step := 0
	if hasEq {
		raw = eqVal
	} else {
		if idx+1 >= len(args) {
			return nil, 0, &ValidationError{
				Code:    "invalid_argument",
				Message: fmt.Sprintf("参数 --%s 缺少取值", name),
				Field:   name,
			}
		}
		raw = args[idx+1]
		step = 1
	}

	switch kind {
	case FlagString:
		return raw, step, nil
	case FlagInt:
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return nil, 0, &ValidationError{
				Code:    "invalid_argument",
				Message: fmt.Sprintf("参数 --%s 需要整数，收到 %q", name, raw),
				Field:   name,
			}
		}
		return n, step, nil
	case FlagFloat:
		f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, 0, &ValidationError{
				Code:    "invalid_argument",
				Message: fmt.Sprintf("参数 --%s 需要有限数值，收到 %q", name, raw),
				Field:   name,
			}
		}
		return f, step, nil
	case FlagCSV:
		return SplitCSV(raw), step, nil
	default:
		return nil, 0, fmt.Errorf("unsupported flag kind %d", kind)
	}
}

func parseStrictBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", raw)
	}
}

func isNegativeNumber(tok string) bool {
	if len(tok) < 2 || tok[0] != '-' {
		return false
	}
	_, err := strconv.ParseFloat(tok, 64)
	return err == nil
}

// SplitCSV parses a comma-separated string into trimmed non-empty elements.
func SplitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// ParseTypedValue parses a CLI scalar or JSON literal into a typed Go value.
func ParseTypedValue(raw string) (any, error) {
	trimmed := strings.TrimSpace(raw)
	switch trimmed {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, `"`) {
		var out any
		dec := json.NewDecoder(strings.NewReader(trimmed))
		dec.UseNumber()
		if err := dec.Decode(&out); err != nil {
			return nil, &ValidationError{
				Code:    "invalid_argument",
				Message: fmt.Sprintf("JSON 值格式错误: %v", err),
			}
		}
		return out, nil
	}
	if n, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return n, nil
	}
	if f, err := strconv.ParseFloat(trimmed, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		return f, nil
	}
	return raw, nil
}

// ParseKeyValues parses "key=value" arguments into a map with typed values.
func ParseKeyValues(args []string) (map[string]any, error) {
	out := make(map[string]any, len(args))
	for _, arg := range args {
		key, rawVal, ok := strings.Cut(arg, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, &ValidationError{
				Code:    "invalid_argument",
				Message: fmt.Sprintf("非法键值参数 %q，格式应为 key=value", arg),
			}
		}
		val, err := ParseTypedValue(rawVal)
		if err != nil {
			return nil, err
		}
		out[key] = val
	}
	return out, nil
}
