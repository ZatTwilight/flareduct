package spec

import (
	"fmt"
	"net/url"

	"flareduct/internal/config"
	"flareduct/internal/strutil"
)

func serviceURL(svc config.ServiceConfig) (string, error) {
	if svc.URL != "" {
		if !strutil.LooksLikeURL(svc.URL) {
			return "", fmt.Errorf("url must include a scheme and host, got %q", svc.URL)
		}
		return svc.URL, nil
	}
	if svc.Port == 0 {
		return "", fmt.Errorf("set url, port, tunnel, or config")
	}
	if svc.Port < 0 || svc.Port > 65535 {
		return "", fmt.Errorf("invalid port %d", svc.Port)
	}
	host := svc.Host
	if host == "" {
		host = "localhost"
	}
	scheme := svc.Scheme
	if scheme == "" {
		scheme = "http"
	}
	return (&url.URL{Scheme: scheme, Host: fmt.Sprintf("%s:%d", host, svc.Port)}).String(), nil
}
