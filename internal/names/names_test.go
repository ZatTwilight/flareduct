package names

import "testing"

func TestSanitizeName(t *testing.T) {
	got := SanitizeName("HTTP://Local Host:3000/foo")
	want := "http-local-host-3000-foo"
	if got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
}
