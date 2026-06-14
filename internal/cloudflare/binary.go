package cloudflare

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func ResolveBinaryPath(cloudflared string) (string, error) {
	path, err := exec.LookPath(cloudflared)
	if err != nil && strings.Contains(cloudflared, string(os.PathSeparator)) {
		if info, statErr := os.Stat(cloudflared); statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			path = cloudflared
			err = nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("cloudflared not found at %q: %w (is cloudflared installed and on PATH?)", cloudflared, err)
	}
	return path, nil
}
