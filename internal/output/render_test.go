package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Yachiiiiyo/xLyra-cli/internal/protocol"
)

func TestProjectEnvelope(t *testing.T) {
	env := protocol.Envelope{
		OK: true,
		Data: map[string]any{
			"items": []any{
				map[string]any{
					"id":      "1",
					"slug":    "openrouter",
					"secret":  "should-hide",
					"gateway": map[string]any{"max_concurrency": 10, "timeout": 5000},
				},
			},
			"count": 1,
		},
	}

	got := ProjectEnvelope(env, []string{"slug", "gateway.max_concurrency"})
	dataMap, ok := got.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map data, got %T", got.Data)
	}
	items := dataMap["items"].([]any)
	first := items[0].(map[string]any)
	if _, hasSecret := first["secret"]; hasSecret {
		t.Fatal("expected unprojected field 'secret' to be dropped")
	}
	if first["slug"] != "openrouter" {
		t.Fatalf("expected slug openrouter, got %v", first["slug"])
	}
	gw := first["gateway"].(map[string]any)
	if len(gw) != 1 || gw["max_concurrency"] != 10 {
		t.Fatalf("unexpected projected gateway: %+v", gw)
	}
}

func TestRenderFormats(t *testing.T) {
	env := protocol.Envelope{
		OK: true,
		Data: []any{
			map[string]any{
				"id":      "s1",
				"name":    "site: one",
				"enabled": true,
				"tags":    []any{"fast", "123"},
			},
		},
		Diff: []protocol.DiffItem{
			{Path: "log.level", Before: "info", After: "debug"},
		},
		Meta: &protocol.Meta{
			Tier:      2,
			DryRun:    true,
			Revision:  "sha256:abc",
			Truncated: false,
		},
	}

	var jsonBuf bytes.Buffer
	if err := Render(&jsonBuf, "json", env); err != nil {
		t.Fatalf("render json: %v", err)
	}
	var roundTrip protocol.Envelope
	if err := json.Unmarshal(jsonBuf.Bytes(), &roundTrip); err != nil {
		t.Fatalf("invalid rendered JSON: %v", err)
	}
	if !roundTrip.OK || len(roundTrip.Diff) != 1 || roundTrip.Diff[0].After != "debug" {
		t.Fatalf("unexpected JSON round-trip: %+v", roundTrip)
	}

	var yamlBuf bytes.Buffer
	if err := Render(&yamlBuf, "yaml", env); err != nil {
		t.Fatalf("render yaml: %v", err)
	}
	yamlText := yamlBuf.String()
	if !strings.Contains(yamlText, "ok: true") || !strings.Contains(yamlText, `"site: one"`) || !strings.Contains(yamlText, `"123"`) {
		t.Fatalf("unexpected YAML output:\n%s", yamlText)
	}

	var tableBuf bytes.Buffer
	if err := Render(&tableBuf, "table", env); err != nil {
		t.Fatalf("render table: %v", err)
	}
	tableText := tableBuf.String()
	if !strings.Contains(tableText, "PATH") || !strings.Contains(tableText, "log.level") || !strings.Contains(tableText, "dry_run=true") {
		t.Fatalf("unexpected table output:\n%s", tableText)
	}
}

func TestRenderTableListAndError(t *testing.T) {
	listEnv := protocol.Envelope{
		OK: true,
		Data: map[string]any{
			"items": []any{
				map[string]any{"id": "1", "slug": "a", "enabled": true},
			},
		},
	}
	var buf bytes.Buffer
	if err := Render(&buf, "table", listEnv); err != nil {
		t.Fatalf("render list table: %v", err)
	}
	if !strings.Contains(buf.String(), "ID") && !strings.Contains(buf.String(), "id") {
		t.Fatalf("expected columns in table:\n%s", buf.String())
	}

	errEnv := protocol.NewErrorEnvelope("policy_denied", "blocked", "agent.settings", "use web ui")
	buf.Reset()
	if err := Render(&buf, "table", errEnv); err != nil {
		t.Fatalf("render error table: %v", err)
	}
	if !strings.Contains(buf.String(), "ERROR [policy_denied]: blocked") || !strings.Contains(buf.String(), "Hint:  use web ui") {
		t.Fatalf("unexpected error table:\n%s", buf.String())
	}
}
