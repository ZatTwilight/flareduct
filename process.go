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

type DetachOptions struct {
	Name    string
	Replace bool
	Wait    time.Duration
}

type ProvisionResult struct {
	CreatedTunnel bool
}

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

	printedURL := spec.PublicURL != ""
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
				if spec.PublicURL != "" {
					fmt.Fprintf(stdout, "flareduct: URL still live at %s\n", spec.PublicURL)
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

func printRunPanel(spec TunnelSpec, logPath string, w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "╭─ flareduct ─────────────────────────────────────────────")
	if spec.PublicURL != "" {
		fmt.Fprintf(w, "│ PUBLIC  %s\n", spec.PublicURL)
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

func ProvisionOwnedTunnel(spec TunnelSpec, stdout, stderr io.Writer) (ProvisionResult, error) {
	var result ProvisionResult
	if spec.Hostname == "" || spec.TunnelName == "" {
		return result, nil
	}
	if len(spec.Command) == 0 {
		return result, fmt.Errorf("empty command")
	}
	cloudflared := spec.Command[0]

	exists, err := tunnelExists(cloudflared, spec.TunnelName)
	if err != nil {
		return result, err
	}
	if exists {
		fmt.Fprintf(stdout, "flareduct: tunnel %s already exists; auto-cleanup will not delete existing resources\n", spec.TunnelName)
	} else {
		createCmd := []string{cloudflared, "tunnel", "create", spec.TunnelName}
		if err := runProvisionCommand("creating tunnel "+spec.TunnelName, createCmd, stdout, stderr, spec.Verbose); err != nil {
			return result, err
		}
		result.CreatedTunnel = true
	}

	routeCmd := []string{cloudflared, "tunnel", "route", "dns"}
	if spec.OverwriteDNS {
		routeCmd = append(routeCmd, "--overwrite-dns")
	}
	routeCmd = append(routeCmd, spec.TunnelName, spec.Hostname)
	if err := runProvisionCommand("routing DNS "+spec.Hostname, routeCmd, stdout, stderr, spec.Verbose); err != nil {
		if result.CreatedTunnel {
			cleanupOwnedTunnelBestEffort(spec, result, stdout, stderr)
		}
		return result, err
	}
	return result, nil
}

func tunnelExists(cloudflared, tunnelName string) (bool, error) {
	cmd := exec.Command(cloudflared, "tunnel", "info", tunnelName)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, err
}

func runProvisionCommand(label string, args []string, stdout, stderr io.Writer, verbose bool) error {
	fmt.Fprintf(stdout, "flareduct: %s\n", label)
	if verbose {
		fmt.Fprintf(stdout, "flareduct: running %s\n", ShellQuote(args))
	}
	cmd := exec.Command(args[0], args[1:]...)
	if verbose {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("provisioning failed for %s: %w", ShellQuote(args), err)
		}
		return nil
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			fmt.Fprint(stderr, string(out))
		}
		return fmt.Errorf("provisioning failed for %s: %w", ShellQuote(args), err)
	}
	return nil
}

func StartDetached(spec TunnelSpec, opts DetachOptions, stdout, stderr io.Writer) error {
	if len(spec.Command) == 0 {
		return fmt.Errorf("empty command")
	}
	if opts.Name == "" {
		opts.Name = spec.Name
	}
	if opts.Wait == 0 {
		opts.Wait = 20 * time.Second
	}
	opts.Name = SanitizeName(opts.Name)
	if opts.Name == "" {
		return fmt.Errorf("invalid detached name")
	}

	state, err := LoadState()
	if err != nil {
		return err
	}
	if entry, _, ok := state.Find(opts.Name); ok && ProcessAlive(entry.PID) {
		if !opts.Replace {
			return fmt.Errorf("%q is already running as pid %d (use --replace or --name)", opts.Name, entry.PID)
		}
		fmt.Fprintf(stdout, "flareduct: replacing %s (pid %d)\n", entry.Name, entry.PID)
		if err := TerminatePID(entry.PID, 5*time.Second); err != nil {
			return err
		}
	}
	state.RemoveName(opts.Name)

	provision, err := ProvisionOwnedTunnel(spec, stdout, stderr)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(LogsDirPath(), 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(LogsDirPath(), fmt.Sprintf("%s-%s.log", opts.Name, time.Now().Format("20060102-150405")))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}

	cmd := exec.Command(spec.Command[0], spec.Command[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		cleanupOwnedTunnelBestEffort(spec, provision, stdout, stderr)
		return err
	}
	pid := cmd.Process.Pid
	_ = logFile.Close()
	if err := cmd.Process.Release(); err != nil {
		return err
	}

	entry := StateEntry{
		Name:          opts.Name,
		Target:        spec.Target,
		Kind:          spec.Kind,
		URL:           spec.URL,
		Hostname:      spec.Hostname,
		TunnelName:    spec.TunnelName,
		PublicURL:     spec.PublicURL,
		AutoCleanup:   spec.AutoCleanup,
		CreatedTunnel: provision.CreatedTunnel,
		Zone:          spec.Zone,
		ZoneID:        spec.ZoneID,
		PID:           pid,
		StartedAt:     time.Now(),
		Command:       spec.Command,
		LogPath:       logPath,
	}
	state.Upsert(entry)
	if err := SaveState(state); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "flareduct: started %s as pid %d\n", opts.Name, pid)
	if spec.Kind == KindQuick && spec.URL != "" {
		fmt.Fprintf(stdout, "flareduct: local service %s\n", spec.URL)
	}
	if entry.PublicURL != "" {
		fmt.Fprintf(stdout, "flareduct: public URL %s\n", entry.PublicURL)
		fmt.Fprintf(stdout, "flareduct: logs %s\n", logPath)
		return nil
	}

	if spec.Kind == KindQuick {
		if publicURL := WaitForPublicURL(logPath, pid, opts.Wait); publicURL != "" {
			entry.PublicURL = publicURL
			state.Upsert(entry)
			_ = SaveState(state)
			fmt.Fprintf(stdout, "flareduct: public URL %s\n", publicURL)
		} else {
			fmt.Fprintf(stderr, "flareduct: no public URL detected within %s; check logs\n", opts.Wait)
		}
	}
	fmt.Fprintf(stdout, "flareduct: logs %s\n", logPath)
	return nil
}

func WaitForPublicURL(logPath string, pid int, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(logPath)
		if err == nil {
			if publicURL := ExtractFirstPublicURLFromBytes(data); publicURL != "" {
				return publicURL
			}
		}
		if !ProcessAlive(pid) {
			return ""
		}
		time.Sleep(250 * time.Millisecond)
	}
	return ""
}
