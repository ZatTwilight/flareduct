package staticserver

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartStaticServerForFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "coolfile.html")
	asset := filepath.Join(dir, "style.css")
	if err := os.WriteFile(file, []byte("<h1>Hello</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(asset, []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	server, err := StartIfPath(file)
	if err != nil {
		t.Fatalf("StartIfPath returned error: %v", err)
	}
	if server == nil {
		t.Fatal("server is nil")
	}
	defer server.Close()

	body := getTestURL(t, server.URL+"/")
	if !strings.Contains(body, "Hello") {
		t.Fatalf("root body = %q", body)
	}
	css := getTestURL(t, server.URL+"/style.css")
	if css != "body{}" {
		t.Fatalf("asset body = %q", css)
	}
}

func TestStartStaticServerIgnoresNonPathTargets(t *testing.T) {
	for _, target := range []string{"3000", "http://localhost:3000", "127.0.0.1:3000", "definitely-not-real"} {
		server, err := StartIfPath(target)
		if err != nil {
			t.Fatalf("%s returned error: %v", target, err)
		}
		if server != nil {
			server.Close()
			t.Fatalf("%s started a server", target)
		}
	}
}

func getTestURL(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
