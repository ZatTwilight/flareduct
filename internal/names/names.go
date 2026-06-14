package names

import (
	"fmt"
	"net/url"
	"strings"

	"flareduct/internal/strutil"
)

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

func DefaultNameForTarget(target string) string {
	if strutil.IsDigits(target) {
		return "port-" + target
	}
	if strutil.LooksLikeURL(target) {
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
