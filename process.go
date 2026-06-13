package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func RunForeground(spec TunnelSpec, stdout, stderr io.Writer) error {
	if len(spec.Command) == 0 {
		return fmt.Errorf("empty command")
	}
	provision, err := ProvisionOwnedTunnel(spec, stdout, stderr)
	if err != nil {
		return err
	}
	defer cleanupOwnedTunnelBestEffort(spec, provision, stdout, stderr)

	logFile, logPath, err := openRunLog(spec.Name)
	if err != nil {
		fmt.Fprintf(stderr, "flareduct: log warning: %v\n", err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	printRunPanel(spec, logPath, stdout)
	if spec.Verbose {
		fmt.Fprintf(stdout, "flareduct: starting %s\n", ShellQuote(spec.Command))
	}

	cmd := exec.Command(spec.Command[0], spec.Command[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmdStdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmdStderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	urlCh := make(chan string, 1)
	statusCh := make(chan string, 4)
	var shuttingDown atomic.Bool
	var streamWG sync.WaitGroup
	streamWG.Add(2)
	go streamCommandOutput(cmdStdout, stdout, logFile, urlCh, statusCh, &streamWG, spec.Verbose, &shuttingDown)
	go streamCommandOutput(cmdStderr, stderr, logFile, urlCh, statusCh, &streamWG, spec.Verbose, &shuttingDown)

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	printedURL := false
	printedConnected := false
	for {
		select {
		case url := <-urlCh:
			if url != "" && !printedURL {
				fmt.Fprintln(stdout)
				printURLBanner(url, stdout)
				printedURL = true
			}
		case status := <-statusCh:
			if !printedConnected {
				fmt.Fprintf(stdout, "flareduct: connected%s\n", status)
				if spec.PublicURL != "" && !printedURL {
					waitForForegroundPublicURL(spec.PublicURL, stdout, stderr)
					printedURL = true
				}
				printedConnected = true
			}
		case sig := <-sigCh:
			shuttingDown.Store(true)
			fmt.Fprintf(stderr, "\nflareduct: received %s, stopping...\n", sig)
			_ = TerminatePID(cmd.Process.Pid, 5*time.Second)
		case err := <-waitCh:
			streamWG.Wait()
			if shuttingDown.Load() {
				fmt.Fprintln(stdout, "flareduct: stopped")
				return nil
			}
			if err != nil {
				if logPath != "" {
					return fmt.Errorf("%w (logs: %s)", err, logPath)
				}
				return err
			}
			return nil
		}
	}
}

func streamCommandOutput(r io.Reader, userOut, logOut io.Writer, urlCh chan<- string, statusCh chan<- string, wg *sync.WaitGroup, verbose bool, shuttingDown *atomic.Bool) {
	defer wg.Done()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if logOut != nil {
			fmt.Fprintln(logOut, line)
		}
		if url := ExtractFirstPublicURL(line); url != "" {
			select {
			case urlCh <- url:
			default:
			}
		}
		if status := connectionStatus(line); status != "" {
			select {
			case statusCh <- status:
			default:
			}
		}
		if verbose {
			fmt.Fprintln(userOut, line)
			continue
		}
		if shuttingDown != nil && shuttingDown.Load() {
			continue
		}
		if shouldShowCloudflaredLine(line) {
			fmt.Fprintf(userOut, "cloudflared: %s\n", line)
		}
	}
}

func openRunLog(name string) (*os.File, string, error) {
	if err := os.MkdirAll(LogsDirPath(), 0o755); err != nil {
		return nil, "", err
	}
	path := filepath.Join(LogsDirPath(), fmt.Sprintf("%s-%s.log", SanitizeName(name), time.Now().Format("20060102-150405")))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, "", err
	}
	return file, path, nil
}

func waitForForegroundPublicURL(publicURL string, stdout, stderr io.Writer) {
	fmt.Fprintf(stdout, "flareduct: checking public URL %s\n", publicURL)
	result := WaitForPublicURLReady(publicURL, publicURLReadinessTimeout)
	if !result.Ready {
		if result.LastError != nil {
			fmt.Fprintf(stderr, "flareduct: public URL still returned errors after %s: %v\n", publicURLReadinessTimeout, result.LastError)
		} else {
			fmt.Fprintf(stderr, "flareduct: public URL still not ready after %s (last status %d)\n", publicURLReadinessTimeout, result.LastStatus)
		}
	}
	fmt.Fprintln(stdout)
	printURLBanner(publicURL, stdout)
}

func printRunPanel(spec TunnelSpec, logPath string, w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "╭─ flareduct ─────────────────────────────────────────────")
	if spec.PublicURL != "" {
		fmt.Fprintf(w, "│ PUBLIC  preparing %s\n", spec.PublicURL)
	} else {
		fmt.Fprintln(w, "│ PUBLIC  waiting for trycloudflare.com URL...")
	}
	if spec.URL != "" {
		fmt.Fprintf(w, "│ LOCAL   %s\n", spec.URL)
	}
	if spec.Hostname != "" {
		fmt.Fprintf(w, "│ HOST    %s\n", spec.Hostname)
	}
	if logPath != "" {
		fmt.Fprintf(w, "│ LOGS    %s\n", logPath)
	}
	fmt.Fprintln(w, "│ STOP    Ctrl-C")
	fmt.Fprintln(w, "╰──────────────────────────────────────────────────────────")
	fmt.Fprintln(w)
}

func printURLBanner(url string, w io.Writer) {
	fmt.Fprintln(w, "╭─ flareduct URL ─────────────────────────────────────────")
	fmt.Fprintf(w, "│ %s\n", url)
	fmt.Fprintln(w, "╰──────────────────────────────────────────────────────────")
}

func connectionStatus(line string) string {
	if !strings.Contains(line, "Registered tunnel connection") {
		return ""
	}
	if location := valueAfter(line, "location="); location != "" {
		return " (" + location + ")"
	}
	return ""
}

func valueAfter(line, key string) string {
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

func shouldShowCloudflaredLine(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "context canceled") || strings.Contains(lower, "application error 0x0") {
		return false
	}
	if strings.Contains(line, " ERR ") || strings.Contains(line, " FTL ") {
		return true
	}
	if strings.Contains(lower, "failed") || strings.Contains(lower, "panic:") {
		return true
	}
	return false
}
