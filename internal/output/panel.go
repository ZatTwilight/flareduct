package output

import (
	"fmt"
	"io"

	"flareduct/internal/spec"
)

func PrintRunPanel(s spec.Spec, logPath string, w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "╭─ flareduct ─────────────────────────────────────────────")
	if s.PublicURL != "" {
		fmt.Fprintf(w, "│ PUBLIC  preparing %s\n", s.PublicURL)
	} else {
		fmt.Fprintln(w, "│ PUBLIC  waiting for trycloudflare.com URL...")
	}
	if s.URL != "" {
		fmt.Fprintf(w, "│ LOCAL   %s\n", s.URL)
	}
	if s.Hostname != "" {
		fmt.Fprintf(w, "│ HOST    %s\n", s.Hostname)
	}
	if logPath != "" {
		fmt.Fprintf(w, "│ LOGS    %s\n", logPath)
	}
	fmt.Fprintln(w, "│ STOP    Ctrl-C")
	fmt.Fprintln(w, "╰──────────────────────────────────────────────────────────")
	fmt.Fprintln(w)
}

func PrintURLBanner(url string, w io.Writer) {
	fmt.Fprintln(w, "╭─ flareduct URL ─────────────────────────────────────────")
	fmt.Fprintf(w, "│ %s\n", url)
	fmt.Fprintln(w, "╰──────────────────────────────────────────────────────────")
}
