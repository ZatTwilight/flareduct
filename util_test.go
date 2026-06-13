package main

import "testing"

func TestExtractFirstPublicURL(t *testing.T) {
	line := "INF Requesting new quick Tunnel on trycloudflare.com... https://alpha-beta.trycloudflare.com"
	got := ExtractFirstPublicURL(line)
	want := "https://alpha-beta.trycloudflare.com"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestSanitizeName(t *testing.T) {
	got := SanitizeName("HTTP://Local Host:3000/foo")
	want := "http-local-host-3000-foo"
	if got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
}
