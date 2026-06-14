package token

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"flareduct/internal/paths"
	"golang.org/x/term"
)

func DefaultTokenPath() string {
	return filepath.Join(filepath.Dir(paths.DefaultConfigPath()), "cloudflare-api-token")
}

func LoadCloudflareAPIToken() (token, source string, ok bool, err error) {
	for _, name := range []string{"CLOUDFLARE_API_TOKEN", "CF_API_TOKEN"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value, "$" + name, true, nil
		}
	}
	for _, name := range []string{"CLOUDFLARE_API_TOKEN_FILE", "CF_API_TOKEN_FILE"} {
		if path := strings.TrimSpace(os.Getenv(name)); path != "" {
			token, err := readTokenFile(path)
			if err != nil {
				return "", "$" + name, false, err
			}
			return token, paths.ExpandPath(path), true, nil
		}
	}

	path := DefaultTokenPath()
	token, err = readTokenFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", path, false, nil
		}
		return "", path, false, err
	}
	return token, path, true, nil
}

func readTokenFile(path string) (string, error) {
	data, err := os.ReadFile(paths.ExpandPath(path))
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file %s is empty", path)
	}
	return token, nil
}

func SaveCloudflareAPIToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("empty token")
	}
	path := DefaultTokenPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	_ = os.Chmod(path, 0o600)
	return path, nil
}

func RemoveCloudflareAPIToken() (string, error) {
	path := DefaultTokenPath()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return path, err
	}
	return path, nil
}

func ReadTokenInteractively(stdin io.Reader, stdout io.Writer) (string, error) {
	if file, ok := stdin.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(stdout, "Cloudflare API token: ")
		data, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(stdout)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("no token on stdin")
	}
	return strings.TrimSpace(scanner.Text()), nil
}
