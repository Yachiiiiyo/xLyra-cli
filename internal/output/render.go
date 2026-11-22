// Package output renders control-plane envelopes in JSON, YAML, or human-readable tables
// and applies client-side fallback --fields projection when requested.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
)

// ProjectEnvelope applies optional client-side --fields filtering on env.Data as a fallback
// in case the backend returned unprojected fields.
func ProjectEnvelope(env protocol.Envelope, fields []string) protocol.Envelope {
	if !env.OK || env.Data == nil || len(fields) == 0 {
		return env
	}
	env.Data = projectValue(env.Data, fields)
	return env
}

func projectValue(data any, fields []string) any {
	switch v := data.(type) {
	case map[string]any:
		// Unwrap standard list wrapper { items: [...], count/total: ... } if items is a slice.
		if items, ok := v["items"].([]any); ok && !containsField(fields, "items") {
			projected := make(map[string]any, len(v))
			for k, val := range v {
				if k == "items" {
					projected[k] = projectValue(items, fields)
				} else {
					projected[k] = val
				}
			}
			return projected
		}
		return projectMap(v, fields)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, projectMap(m, fields))
			} else {
				out = append(out, item)
			}
		}
		return out
	default:
		return data
	}
}

func containsField(fields []string, target string) bool {
	for _, f := range fields {
		if f == target {
			return true
		}
	}
	return false
}

func projectMap(src map[string]any, fields []string) map[string]any {
	dst := make(map[string]any)
	for _, field := range fields {
		parts := strings.Split(field, ".")
		copyPath(dst, src, parts)
	}
	return dst
}

func copyPath(dst, src map[string]any, parts []string) {
	if len(parts) == 0 {
		return
	}
	key := parts[0]
	val, ok := src[key]
	if !ok {
		return
	}
	if len(parts) == 1 {
		dst[key] = val
		return
	}
	subSrc, ok := val.(map[string]any)
	if !ok {
		return
	}
	subDst, ok := dst[key].(map[string]any)
	if !ok {
		subDst = make(map[string]any)
		dst[key] = subDst
	}
	copyPath(subDst, subSrc, parts[1:])
}

// Render writes the envelope to w in json, yaml, or table format.
func Render(w io.Writer, format string, env protocol.Envelope) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return renderJSON(w, env)
	case "yaml":
		return renderYAML(w, env)
	case "table":
		return renderTable(w, env)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func renderJSON(w io.Writer, env protocol.Envelope) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}

func renderYAML(w io.Writer, env protocol.Envelope) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	var generic any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&generic); err != nil {
		return err
	}
	var b strings.Builder
	writeYAML(&b, generic, 0)
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteByte('\n')
	}
	_, err = io.WriteString(w, b.String())
	return err
}

func writeYAML(b *strings.Builder, val any, indent int) {
	prefix := strings.Repeat("  ", indent)
	switch v := val.(type) {
	case nil:
		b.WriteString("null\n")
	case bool:
		if v {
			b.WriteString("true\n")
		} else {
			b.WriteString("false\n")
		}
	case json.Number:
		b.WriteString(v.String())
		b.WriteByte('\n')
	case float64:
		b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		b.WriteByte('\n')
	case int64:
		b.WriteString(strconv.FormatInt(v, 10))
		b.WriteByte('\n')
	case string:
		b.WriteString(yamlScalarString(v))
		b.WriteByte('\n')
	case []any:
		if len(v) == 0 {
			b.WriteString("[]\n")
			return
		}
		for _, item := range v {
			switch child := item.(type) {
			case map[string]any:
				if len(child) == 0 {
					b.WriteString(prefix + "- {}\n")
					continue
				}
				keys := sortedKeys(child)
				firstKey := keys[0]
				firstVal := child[firstKey]
				b.WriteString(prefix + "- " + yamlKeyString(firstKey) + ":")
				if isYAMLComposite(firstVal) {
					b.WriteByte('\n')
					writeYAML(b, firstVal, indent+2)
				} else {
					b.WriteByte(' ')
					writeYAML(b, firstVal, indent+2)
				}
				for _, k := range keys[1:] {
					b.WriteString(prefix + "  " + yamlKeyString(k) + ":")
					if isYAMLComposite(child[k]) {
						b.WriteByte('\n')
						writeYAML(b, child[k], indent+2)
					} else {
						b.WriteByte(' ')
						writeYAML(b, child[k], indent+2)
					}
				}
			case []any:
				if len(child) == 0 {
					b.WriteString(prefix + "- []\n")
				} else {
					b.WriteString(prefix + "-\n")
					writeYAML(b, child, indent+1)
				}
			default:
				b.WriteString(prefix + "- ")
				writeYAML(b, item, indent+1)
			}
		}
	case map[string]any:
		if len(v) == 0 {
			b.WriteString("{}\n")
			return
		}
		for _, k := range sortedKeys(v) {
			b.WriteString(prefix + yamlKeyString(k) + ":")
			if isYAMLComposite(v[k]) {
				b.WriteByte('\n')
				writeYAML(b, v[k], indent+1)
			} else {
				b.WriteByte(' ')
				writeYAML(b, v[k], indent+1)
			}
		}
	default:
		b.WriteString(yamlScalarString(fmt.Sprint(v)))
		b.WriteByte('\n')
	}
}

func isYAMLComposite(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		return len(x) > 0
	case []any:
		return len(x) > 0
	default:
		return false
	}
}

func yamlKeyString(k string) string {
	if needsYAMLQuotes(k) || strings.ContainsAny(k, ":\n\r\t") {
		return strconv.Quote(k)
	}
	return k
}

func yamlScalarString(s string) string {
	if needsYAMLQuotes(s) {
		return strconv.Quote(s)
	}
	return s
}

func needsYAMLQuotes(s string) bool {
	if s == "" || strings.TrimSpace(s) != s {
		return true
	}
	switch strings.ToLower(s) {
	case "true", "false", "null", "~", "yes", "no", "on", "off":
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	if strings.ContainsAny(s, "\n\r\t\"'\\") || strings.Contains(s, ": ") || strings.Contains(s, " #") {
		return true
	}
	switch s[0] {
	case '-', '?', ':', ',', '[', ']', '{', '}', '#', '&', '*', '!', '|', '>', '%', '@', '`':
		return true
	}
	return false
}

func renderTable(w io.Writer, env protocol.Envelope) error {
	if !env.OK && env.Error != nil {
		var b strings.Builder
		fmt.Fprintf(&b, "ERROR [%s]: %s\n", env.Error.Code, env.Error.Message)
		if env.Error.Field != "" {
			fmt.Fprintf(&b, "Field: %s\n", env.Error.Field)
		}
		if env.Error.Hint != "" {
			fmt.Fprintf(&b, "Hint:  %s\n", env.Error.Hint)
		}
		_, err := io.WriteString(w, b.String())
		return err
	}

	if len(env.Diff) > 0 {
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "PATH\tBEFORE\tAFTER")
		for _, d := range env.Diff {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", d.Path, formatCell(d.Before), formatCell(d.After))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		return writeTableMetaFooter(w, env.Meta)
	}

	normalized, err := toGenericJSON(env.Data)
	if err != nil {
		return err
	}

	switch data := normalized.(type) {
	case nil:
		_, _ = fmt.Fprintln(w, "OK")
	case []any:
		if err := writeSliceTable(w, data); err != nil {
			return err
		}
	case map[string]any:
		if items, ok := data["items"].([]any); ok {
			if err := writeSliceTable(w, items); err != nil {
				return err
			}
		} else {
			tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "KEY\tVALUE")
			for _, k := range sortedKeys(data) {
				_, _ = fmt.Fprintf(tw, "%s\t%s\n", k, formatCell(data[k]))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
	default:
		_, _ = fmt.Fprintln(w, formatCell(data))
	}

	return writeTableMetaFooter(w, env.Meta)
}

func writeSliceTable(w io.Writer, items []any) error {
	if len(items) == 0 {
		_, _ = fmt.Fprintln(w, "(empty)")
		return nil
	}
	rows := make([]map[string]any, 0, len(items))
	colSet := make(map[string]bool)
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			for _, elem := range items {
				_, _ = fmt.Fprintln(w, formatCell(elem))
			}
			return nil
		}
		rows = append(rows, m)
		for k := range m {
			colSet[k] = true
		}
	}
	cols := orderedTableColumns(colSet)
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(cols, "\t"))
	for _, row := range rows {
		vals := make([]string, len(cols))
		for i, col := range cols {
			vals[i] = formatCell(row[col])
		}
		_, _ = fmt.Fprintln(tw, strings.Join(vals, "\t"))
	}
	return tw.Flush()
}

func orderedTableColumns(colSet map[string]bool) []string {
	priority := []string{"id", "slug", "name", "model_key", "display_name", "status", "enabled", "type", "tier"}
	cols := make([]string, 0, len(colSet))
	used := make(map[string]bool, len(colSet))
	for _, p := range priority {
		if colSet[p] {
			cols = append(cols, p)
			used[p] = true
		}
	}
	var rest []string
	for k := range colSet {
		if !used[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(cols, rest...)
}

func writeTableMetaFooter(w io.Writer, meta *protocol.Meta) error {
	if meta == nil {
		return nil
	}
	var parts []string
	if meta.DryRun {
		parts = append(parts, "dry_run=true")
	}
	if meta.Revision != "" {
		parts = append(parts, "revision="+meta.Revision)
	}
	if meta.Truncated {
		parts = append(parts, "truncated=true")
	}
	if meta.NextPage != nil {
		parts = append(parts, fmt.Sprintf("next_page=%v", meta.NextPage))
	}
	if len(parts) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w, "\n# %s\n", strings.Join(parts, " "))
	return err
}

func toGenericJSON(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func formatCell(v any) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case string:
		return strings.ReplaceAll(x, "\n", "\\n")
	case json.Number:
		return x.String()
	case bool:
		if x {
			return "true"
		}
		return "false"
	case []any, map[string]any:
		raw, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(raw)
	default:
		return fmt.Sprint(x)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
