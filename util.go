package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var tryCloudflareURLRE = regexp.MustCompile(`https://[A-Za-z0-9.-]+\.trycloudflare\.com`)

func HomeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}

func ExpandPath(path string) string {
	if path == "" {
		return path
	}
	path = os.ExpandEnv(path)
	if path == "~" {
		return HomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(HomeDir(), path[2:])
	}
	return path
}

func IsDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func LooksLikeURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func HostPortLike(s string) bool {
	if strings.Contains(s, "://") {
		return false
	}
	idx := strings.LastIndex(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return false
	}
	port := s[idx+1:]
	if !IsDigits(port) {
		return false
	}
	p, err := strconv.Atoi(port)
	return err == nil && p > 0 && p <= 65535
}

func SanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if allowed {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-._")
	if out == "" {
		out = "tunnel"
	}
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-._")
	}
	if out == "" {
		return "tunnel"
	}
	return out
}

func DefaultNameForTarget(target string) string {
	if IsDigits(target) {
		return "port-" + target
	}
	if LooksLikeURL(target) {
		if u, err := url.Parse(target); err == nil && u.Host != "" {
			name := u.Host
			if u.Path != "" && u.Path != "/" {
				name += "-" + strings.Trim(u.Path, "/")
			}
			return SanitizeName(name)
		}
	}
	return SanitizeName(target)
}

func SanitizeDNSLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if allowed {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "tunnel"
	}
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	if out == "" {
		return "tunnel"
	}
	return out
}

func NormalizeHostname(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", fmt.Errorf("empty hostname")
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", err
		}
		if u.Port() != "" {
			return "", fmt.Errorf("hostname must not include a port")
		}
		s = u.Hostname()
	}
	s = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
	if strings.ContainsAny(s, "/:@") {
		return "", fmt.Errorf("hostname must not include a scheme, path, user, or port")
	}
	if len(s) > 253 {
		return "", fmt.Errorf("hostname is too long")
	}
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("hostname must include a domain, got %q", input)
	}
	for _, label := range labels {
		if err := validateDNSLabel(label); err != nil {
			return "", fmt.Errorf("invalid hostname %q: %w", input, err)
		}
	}
	return s, nil
}

func NormalizeSubdomain(input string) (string, error) {
	s := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(input), "."))
	if s == "" {
		return "", fmt.Errorf("empty subdomain")
	}
	if strings.Contains(s, "://") || strings.ContainsAny(s, "/:@") {
		return "", fmt.Errorf("subdomain must be DNS labels only; use --hostname for a full hostname")
	}
	labels := strings.Split(s, ".")
	for _, label := range labels {
		if err := validateDNSLabel(label); err != nil {
			return "", fmt.Errorf("invalid subdomain %q: %w", input, err)
		}
	}
	return s, nil
}

func validateDNSLabel(label string) error {
	if label == "" {
		return fmt.Errorf("empty label")
	}
	if len(label) > 63 {
		return fmt.Errorf("label %q is longer than 63 characters", label)
	}
	for i, r := range label {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !allowed {
			return fmt.Errorf("label %q contains invalid character %q", label, r)
		}
		if (i == 0 || i == len(label)-1) && r == '-' {
			return fmt.Errorf("label %q must not start or end with '-'", label)
		}
	}
	return nil
}

func RandomHexSuffix(byteLen int) string {
	if byteLen <= 0 {
		byteLen = 3
	}
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())[:byteLen*2]
	}
	return hex.EncodeToString(buf)
}

func ExtractFirstPublicURL(line string) string {
	return tryCloudflareURLRE.FindString(line)
}

func ExtractFirstPublicURLFromBytes(data []byte) string {
	match := tryCloudflareURLRE.Find(data)
	if match == nil {
		return ""
	}
	return string(match)
}

func ShellQuote(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "" {
			quoted = append(quoted, "''")
			continue
		}
		if strings.IndexFunc(arg, func(r rune) bool {
			return !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-._/:=+,@%", r))
		}) == -1 {
			quoted = append(quoted, arg)
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
	}
	return strings.Join(quoted, " ")
}

func PrintLinesTail(path string, n int, out io.Writer) error {
	if n < 0 {
		n = 80
	}
	if n == 0 {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			copy(lines, lines[1:])
			lines = lines[:n]
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	return nil
}

func FollowFile(path string, out io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			fmt.Fprint(out, line)
		}
		if err == nil {
			continue
		}
		if err != io.EOF {
			return err
		}
		if _, err := file.Seek(0, io.SeekCurrent); err != nil {
			return err
		}
		// Cheap polling avoids extra dependencies and is enough for log following.
		SleepBriefly()
	}
}

func SleepBriefly() {
	// Defined here to keep log following testable in the future.
	time.Sleep(500 * time.Millisecond)
}
