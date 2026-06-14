package process

import (
	"bufio"
	"errors"
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

	"flareduct/internal/names"
	"flareduct/internal/output"
	"flareduct/internal/paths"
	"flareduct/internal/provision"
	"flareduct/internal/readiness"
	"flareduct/internal/spec"
	"flareduct/internal/strutil"
)

func RunForeground(s spec.Spec, stdout, stderr io.Writer) error {
	if len(s.Command) == 0 {
		return fmt.Errorf("empty command")
	}
	result, err := provision.OwnedTunnel(s, stdout, stderr)
	if err != nil {
		return err
	}
	defer provision.Cleanup(s, result, stdout, stderr)

	logFile, logPath, err := openRunLog(s.Name)
	if err != nil {
		fmt.Fprintf(stderr, "flareduct: log warning: %v\n", err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	output.PrintRunPanel(s, logPath, stdout)
	if s.Verbose {
		fmt.Fprintf(stdout, "flareduct: starting %s\n", strutil.ShellQuote(s.Command))
	}

	cmd := exec.Command(s.Command[0], s.Command[1:]...)
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
	go streamOutput(cmdStdout, stdout, logFile, urlCh, statusCh, &streamWG, s.Verbose, &shuttingDown)
	go streamOutput(cmdStderr, stderr, logFile, urlCh, statusCh, &streamWG, s.Verbose, &shuttingDown)

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
				output.PrintURLBanner(url, stdout)
				printedURL = true
			}
		case status := <-statusCh:
			if !printedConnected {
				fmt.Fprintf(stdout, "flareduct: connected%s\n", status)
				if s.PublicURL != "" && !printedURL {
					waitForForegroundPublicURL(s.PublicURL, stdout, stderr)
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

func streamOutput(r io.Reader, userOut, logOut io.Writer, urlCh chan<- string, statusCh chan<- string, wg *sync.WaitGroup, verbose bool, shuttingDown *atomic.Bool) {
	defer wg.Done()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if logOut != nil {
			fmt.Fprintln(logOut, line)
		}
		if url := strutil.ExtractFirstPublicURL(line); url != "" {
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
	if err := os.MkdirAll(paths.LogsDirPath(), 0o755); err != nil {
		return nil, "", err
	}
	path := filepath.Join(paths.LogsDirPath(), fmt.Sprintf("%s-%s.log", names.SanitizeName(name), time.Now().Format("20060102-150405")))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, "", err
	}
	return file, path, nil
}

func waitForForegroundPublicURL(publicURL string, stdout, stderr io.Writer) {
	fmt.Fprintf(stdout, "flareduct: checking public URL %s\n", publicURL)
	result := readiness.WaitForReady(publicURL, 90*time.Second)
	if !result.Ready {
		if result.LastError != nil {
			fmt.Fprintf(stderr, "flareduct: public URL still returned errors after %s: %v\n", 90*time.Second, result.LastError)
		} else {
			fmt.Fprintf(stderr, "flareduct: public URL still not ready after %s (last status %d)\n", 90*time.Second, result.LastStatus)
		}
	}
	fmt.Fprintln(stdout)
	output.PrintURLBanner(publicURL, stdout)
}

func connectionStatus(line string) string {
	if !strings.Contains(line, "Registered tunnel connection") {
		return ""
	}
	if location := strutil.ValueAfter(line, "location="); location != "" {
		return " (" + location + ")"
	}
	return ""
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

func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func TerminatePID(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	if !ProcessAlive(pid) {
		return nil
	}

	if err := signalProcessGroupOrPID(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !ProcessAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := signalProcessGroupOrPID(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func signalProcessGroupOrPID(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pid, sig); err == nil || !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return syscall.Kill(pid, sig)
}
