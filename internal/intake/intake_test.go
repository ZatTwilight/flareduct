package intake

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flareduct/internal/config"
	"flareduct/internal/spec"
)

func TestPrepareAliasWinsOverSameNamedPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "api"), []byte("not the alias"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	cfg := config.DefaultConfig()
	cfg.Services["api"] = config.ServiceConfig{Port: 8080}
	var stdout bytes.Buffer
	s, cleanup, err := Prepare("api", cfg, Options{}, &stdout)
	if cleanup != nil {
		defer cleanup()
		t.Fatal("alias target unexpectedly returned static server cleanup")
	}
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if s.Kind != spec.KindQuick || s.URL != "http://localhost:8080" || s.Target != "api" {
		t.Fatalf("spec = %#v", s)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want no static serving message", stdout.String())
	}
}

func TestPrepareRejectsStaticDetach(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "page.html")
	if err := os.WriteFile(file, []byte("<h1>Hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	_, cleanup, err := Prepare(file, config.DefaultConfig(), Options{Detach: true}, &stdout)
	if cleanup != nil {
		defer cleanup()
		t.Fatal("detach rejection should close the static server before returning")
	}
	if err == nil || err.Error() != "--detach is not supported for static file targets yet" {
		t.Fatalf("err = %v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want no serving message", stdout.String())
	}
}

func TestPrepareStartsStaticServerForPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "page.html")
	if err := os.WriteFile(file, []byte("<h1>Hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	s, cleanup, err := Prepare(file, config.DefaultConfig(), Options{}, &stdout)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("static target did not return cleanup")
	}
	if s.Kind != spec.KindQuick || s.URL == "" || !strings.HasPrefix(s.URL, "http://127.0.0.1:") {
		t.Fatalf("spec = %#v", s)
	}
	if !strings.Contains(stdout.String(), "flareduct: serving file ") || !strings.Contains(stdout.String(), " at "+s.URL) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
