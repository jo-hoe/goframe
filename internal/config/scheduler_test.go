package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadImageListConfig_WithHeaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image-scheduler.yaml")
	content := `
goframeURL: http://goframe:8080
sourceName: natgeo
source: imagelist
headers:
  - name: User-Agent
    value: "Mozilla/5.0 (compatible)"
  - name: Referer
    value: "https://example.com"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := LoadImageListConfig(path)
	if err != nil {
		t.Fatalf("LoadImageListConfig failed: %v", err)
	}
	if cfg.Source != "imagelist" {
		t.Errorf("expected source imagelist, got %q", cfg.Source)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default logLevel info, got %q", cfg.LogLevel)
	}
	if len(cfg.Headers) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(cfg.Headers))
	}
	if cfg.Headers[0].Name != "User-Agent" || cfg.Headers[0].Value != "Mozilla/5.0 (compatible)" {
		t.Errorf("unexpected first header: %+v", cfg.Headers[0])
	}
}

func TestLoadImageListConfig_NoHeaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image-scheduler.yaml")
	content := "goframeURL: http://goframe:8080\nsourceName: natgeo\nsource: imagelist\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := LoadImageListConfig(path)
	if err != nil {
		t.Fatalf("LoadImageListConfig failed: %v", err)
	}
	if len(cfg.Headers) != 0 {
		t.Errorf("expected no headers, got %d", len(cfg.Headers))
	}
}

func TestHeaderMap(t *testing.T) {
	if got := HeaderMap(nil); got != nil {
		t.Errorf("expected nil for no headers, got %v", got)
	}

	headers := []HTTPHeader{
		{Name: "User-Agent", Value: "ua"},
		{Name: "", Value: "skipped"}, // empty name is ignored
		{Name: "X-Custom", Value: "c"},
	}
	m := HeaderMap(headers)
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d (%v)", len(m), m)
	}
	if m["User-Agent"] != "ua" || m["X-Custom"] != "c" {
		t.Errorf("unexpected map contents: %v", m)
	}
	if _, ok := m[""]; ok {
		t.Error("empty header name should be skipped")
	}
}
