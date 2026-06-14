package strutil

import "testing"

func TestExtractFirstPublicURL(t *testing.T) {
	line := "INF Requesting new quick Tunnel on trycloudflare.com... https://alpha-beta.trycloudflare.com"
	got := ExtractFirstPublicURL(line)
	want := "https://alpha-beta.trycloudflare.com"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}
