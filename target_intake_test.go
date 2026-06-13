package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareUpTargetAliasWinsOverSameNamedPath(t *testing.T) {
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

	cfg := DefaultConfig()
	cfg.Services["api"] = ServiceConfig{Port: 8080}
	var stdout bytes.Buffer
	spec, cleanup, err := PrepareUpTarget("api", cfg, TargetIntakeOptions{}, &stdout)
	if cleanup != nil {
		defer cleanup()
		t.Fatal("alias target unexpectedly returned static server cleanup")
	}
	if err != nil {
		t.Fatalf("PrepareUpTarget returned error: %v", err)
	}
	if spec.Kind != KindQuick || spec.URL != "http://localhost:8080" || spec.Target != "api" {
		t.Fatalf("spec = %#v", spec)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want no static serving message", stdout.String())
	}
}

func TestPrepareUpTargetRejectsStaticDetach(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "page.html")
	if err := os.WriteFile(file, []byte("<h1>Hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	_, cleanup, err := PrepareUpTarget(file, DefaultConfig(), TargetIntakeOptions{Detach: true}, &stdout)
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

func TestPrepareUpTargetStartsStaticServerForPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "page.html")
	if err := os.WriteFile(file, []byte("<h1>Hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	spec, cleanup, err := PrepareUpTarget(file, DefaultConfig(), TargetIntakeOptions{}, &stdout)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("PrepareUpTarget returned error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("static target did not return cleanup")
	}
	if spec.Kind != KindQuick || spec.URL == "" || !strings.HasPrefix(spec.URL, "http://127.0.0.1:") {
		t.Fatalf("spec = %#v", spec)
	}
	if !strings.Contains(stdout.String(), "flareduct: serving file ") || !strings.Contains(stdout.String(), " at "+spec.URL) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
