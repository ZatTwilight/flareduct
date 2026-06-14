package strutil

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var tryCloudflareURLRE = regexp.MustCompile(`https://[A-Za-z0-9.-]+\.trycloudflare\.com`)

func IsDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
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

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ValueAfter(line, key string) string {
	idx := strings.Index(line, key)
	if idx < 0 {
		return ""
	}
	value := line[idx+len(key):]
	if end := strings.IndexAny(value, " \t"); end >= 0 {
		value = value[:end]
	}
	return strings.TrimSpace(value)
}

func FormatErrorf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
