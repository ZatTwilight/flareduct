package staticserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"flareduct/internal/names"
	"flareduct/internal/paths"
	"flareduct/internal/strutil"
)

type Server struct {
	Name string
	URL  string
	Path string
	Kind string
	srv  *http.Server
	ln   net.Listener
}

func StartIfPath(target string) (*Server, error) {
	if target == "" || strutil.LooksLikeURL(target) || strutil.IsDigits(target) || strutil.HostPortLike(target) {
		return nil, nil
	}
	p := paths.ExpandPath(target)
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file or directory", target)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	name := serverName(abs, info)
	handler := fileHandler(abs, info)
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ss := &Server{
		Name: name,
		URL:  "http://" + ln.Addr().String(),
		Path: abs,
		Kind: "directory",
		srv:  srv,
		ln:   ln,
	}
	if !info.IsDir() {
		ss.Kind = "file"
	}

	go func() {
		_ = srv.Serve(ln)
	}()
	return ss, nil
}

func (s *Server) Close() error {
	if s == nil || s.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.srv.Shutdown(ctx)
}

func serverName(abs string, info os.FileInfo) string {
	base := filepath.Base(abs)
	if info.IsDir() {
		if base == "." || base == string(filepath.Separator) || base == "" {
			if wd, err := os.Getwd(); err == nil {
				base = filepath.Base(wd)
			}
		}
		return names.SanitizeName("static-" + base)
	}
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		name = base
	}
	return names.SanitizeName("static-" + name)
}

func fileHandler(abs string, info os.FileInfo) http.Handler {
	if info.IsDir() {
		return http.FileServer(http.Dir(abs))
	}
	parent := filepath.Dir(abs)
	filename := filepath.Base(abs)
	files := http.FileServer(http.Dir(parent))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		if clean == "/" || clean == "/"+filename {
			http.ServeFile(w, r, abs)
			return
		}
		files.ServeHTTP(w, r)
	})
}
