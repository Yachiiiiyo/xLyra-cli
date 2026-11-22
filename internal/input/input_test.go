package input

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgsMixedPositionsAndTypes(t *testing.T) {
	specs := []FlagSpec{
		{Name: "enabled", Kind: FlagBool},
		{Name: "limit", Kind: FlagInt},
		{Name: "amount", Kind: FlagFloat},
		{Name: "ids", Kind: FlagCSV},
		{Name: "debug", Kind: FlagBool},
	}

	parsed, err := ParseArgs([]string{
		"--format", "yaml",
		"site-1",
		"--enabled", "false",
		"--limit=25",
		"--amount", "12.5",
		"--ids", "a, b ,c",
		"--debug",
		"--dry-run",
		"extra-pos",
	}, specs)
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}

	if parsed.Global.Format != "yaml" || !parsed.Global.DryRun {
		t.Fatalf("unexpected global flags: %+v", parsed.Global)
	}
	if len(parsed.Positionals) != 2 || parsed.Positionals[0] != "site-1" || parsed.Positionals[1] != "extra-pos" {
		t.Fatalf("unexpected positionals: %v", parsed.Positionals)
	}
	if b, ok := parsed.Bool("enabled"); !ok || b {
		t.Fatalf("expected enabled=false, got (%v, %v)", b, ok)
	}
	if b, ok := parsed.Bool("debug"); !ok || !b {
		t.Fatalf("expected debug=true, got (%v, %v)", b, ok)
	}
	if n, ok := parsed.Int("limit"); !ok || n != 25 {
		t.Fatalf("expected limit=25, got (%v, %v)", n, ok)
	}
	if f, ok := parsed.Float("amount"); !ok || f != 12.5 {
		t.Fatalf("expected amount=12.5, got (%v, %v)", f, ok)
	}
	if ids, ok := parsed.Strings("ids"); !ok || len(ids) != 3 || ids[1] != "b" {
		t.Fatalf("expected ids=[a b c], got (%v, %v)", ids, ok)
	}
}

func TestParseArgsAllowsNegativePositionalAfterDashDash(t *testing.T) {
	parsed, err := ParseArgs([]string{"set", "foo", "-10"}, nil)
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if len(parsed.Positionals) != 3 || parsed.Positionals[2] != "-10" {
		t.Fatalf("unexpected positionals: %v", parsed.Positionals)
	}
}

func TestParseKeyValuesAndTypedValues(t *testing.T) {
	kv, err := ParseKeyValues([]string{
		"enabled=true",
		"max_concurrency=16",
		"ratio=0.85",
		"tags=[\"a\",\"b\"]",
		"label=hello",
	})
	if err != nil {
		t.Fatalf("ParseKeyValues: %v", err)
	}
	if kv["enabled"] != true || kv["max_concurrency"] != int64(16) || kv["ratio"] != 0.85 || kv["label"] != "hello" {
		t.Fatalf("unexpected parsed kv: %+v", kv)
	}
	tags, ok := kv["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Fatalf("expected []any tags, got %#v", kv["tags"])
	}
}

func TestLoadPayloadFileBoundaryChecks(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()

	outsideFile := filepath.Join(outside, "secret.json")
	if err := os.WriteFile(outsideFile, []byte(`{"secret":1}`), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	insideFile := filepath.Join(ws, "ok.json")
	if err := os.WriteFile(insideFile, []byte(`{"enabled":true}`), 0o600); err != nil {
		t.Fatalf("write inside file: %v", err)
	}
	symlinkEscape := filepath.Join(ws, "escape.json")
	if err := os.Symlink(outsideFile, symlinkEscape); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(ws); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	val, has, err := LoadPayload(GlobalFlags{File: "ok.json"}, nil)
	if err != nil || !has {
		t.Fatalf("expected valid workspace file, got val=%v has=%v err=%v", val, has, err)
	}

	for _, bad := range []string{"~/.ssh/id_rsa", "file:///etc/passwd", outsideFile, "../" + filepath.Base(outside) + "/secret.json", "escape.json"} {
		_, _, err := LoadPayload(GlobalFlags{File: bad}, nil)
		var vErr *ValidationError
		if !errors.As(err, &vErr) || vErr.Code != "policy_denied" {
			t.Fatalf("expected policy_denied for %q, got %v", bad, err)
		}
	}

	_, _, err = LoadPayload(GlobalFlags{JSON: `{"a":1}`, Stdin: true}, strings.NewReader(`{"b":2}`))
	if err == nil {
		t.Fatal("expected error when both --json and --stdin are set")
	}
}

func TestSensitiveCredentialGuards(t *testing.T) {
	if err := ValidateProxyURLNoPassword("http://127.0.0.1:7890"); err != nil {
		t.Fatalf("expected clean proxy URL to pass, got %v", err)
	}
	for _, badURL := range []string{"http://user:pass@proxy.local:8080", "socks5://:secret@127.0.0.1:1080"} {
		var vErr *ValidationError
		if err := ValidateProxyURLNoPassword(badURL); !errors.As(err, &vErr) || vErr.Code != "policy_denied" {
			t.Fatalf("expected policy_denied for %q, got %v", badURL, err)
		}
	}

	okPayload := map[string]any{
		"request_headers": []any{
			map[string]any{"name": "X-Custom-Source", "value": "xlyra"},
		},
	}
	if err := ValidateNoSensitiveRequestHeaders(okPayload); err != nil {
		t.Fatalf("expected safe request header to pass, got %v", err)
	}

	for _, header := range []string{"Authorization", "X-API-Key", "Cookie", "Proxy-Authorization", "X-Custom-Secret"} {
		badPayload := map[string]any{
			"request_headers": []any{
				map[string]any{"name": header, "value": "sk-test"},
			},
		}
		var vErr *ValidationError
		if err := ValidateNoSensitiveRequestHeaders(badPayload); !errors.As(err, &vErr) || vErr.Code != "policy_denied" {
			t.Fatalf("expected policy_denied for header %q, got %v", header, err)
		}
	}
}
