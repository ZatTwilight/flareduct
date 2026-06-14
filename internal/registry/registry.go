package registry

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"flareduct/internal/names"
	"flareduct/internal/output"
	"flareduct/internal/paths"
	"flareduct/internal/process"
	"flareduct/internal/provision"
	"flareduct/internal/readiness"
	"flareduct/internal/spec"
	"flareduct/internal/state"
	"flareduct/internal/strutil"
)

type Options struct {
	Name    string
	Replace bool
	Wait    time.Duration
}

type Registry struct {
	state state.State
}

func Load() (*Registry, error) {
	s, err := state.Load()
	if err != nil {
		return nil, err
	}
	return &Registry{state: s}, nil
}

func (r *Registry) Save() error {
	return state.Save(r.state)
}

func List(stdout io.Writer) error {
	registry, err := Load()
	if err != nil {
		return err
	}
	return registry.list(stdout)
}

func (r *Registry) list(stdout io.Writer) error {
	if len(r.state.Entries) == 0 {
		fmt.Fprintln(stdout, "No detached tunnels.")
		return nil
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPID\tSTATUS\tKIND\tTARGET\tPUBLIC URL\tLOG")
	for _, entry := range r.state.Entries {
		status := "stopped"
		if process.ProcessAlive(entry.PID) {
			status = "running"
		}
		publicURL := entry.PublicURL
		if publicURL == "" {
			publicURL = "-"
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n", entry.Name, entry.PID, status, entry.Kind, entry.Target, publicURL, entry.LogPath)
	}
	return tw.Flush()
}

func Stop(key string, all bool, stdout, stderr io.Writer) error {
	registry, err := Load()
	if err != nil {
		return err
	}
	return registry.stop(key, all, stdout, stderr)
}

func (r *Registry) stop(key string, all bool, stdout, stderr io.Writer) error {
	if len(r.state.Entries) == 0 {
		fmt.Fprintln(stdout, "No detached tunnels.")
		return nil
	}

	if all {
		for _, entry := range r.state.Entries {
			if process.ProcessAlive(entry.PID) {
				fmt.Fprintf(stdout, "Stopping %s (pid %d)\n", entry.Name, entry.PID)
				if err := process.TerminatePID(entry.PID, 5*time.Second); err != nil {
					return err
				}
			}
			provision.CleanupBestEffort(provision.SpecFromEntry(entry), provision.Result{CreatedTunnel: entry.CreatedTunnel}, stdout, stderr)
		}
		r.state.Entries = nil
		return r.Save()
	}

	entry, idx, ok := r.state.Find(key)
	if !ok {
		return fmt.Errorf("no detached tunnel named/pid %q", key)
	}
	if process.ProcessAlive(entry.PID) {
		fmt.Fprintf(stdout, "Stopping %s (pid %d)\n", entry.Name, entry.PID)
		if err := process.TerminatePID(entry.PID, 5*time.Second); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(stdout, "%s was already stopped\n", entry.Name)
	}
	provision.CleanupBestEffort(provision.SpecFromEntry(entry), provision.Result{CreatedTunnel: entry.CreatedTunnel}, stdout, stderr)
	r.state.RemoveIndex(idx)
	return r.Save()
}

func ShowLogs(key string, lines int, follow bool, stdout io.Writer) error {
	registry, err := Load()
	if err != nil {
		return err
	}
	return registry.logs(key, lines, follow, stdout)
}

func (r *Registry) logs(key string, lines int, follow bool, stdout io.Writer) error {
	entry, _, ok := r.state.Find(key)
	if !ok {
		return fmt.Errorf("no detached tunnel named/pid %q", key)
	}
	if entry.LogPath == "" {
		return fmt.Errorf("no log path stored for %q", entry.Name)
	}
	if err := output.PrintLinesTail(entry.LogPath, lines, stdout); err != nil {
		return err
	}
	if follow {
		return output.FollowFile(entry.LogPath, stdout)
	}
	return nil
}

func Start(s spec.Spec, opts Options, stdout, stderr io.Writer) error {
	if len(s.Command) == 0 {
		return fmt.Errorf("empty command")
	}
	if opts.Name == "" {
		opts.Name = s.Name
	}
	if opts.Wait == 0 {
		opts.Wait = 20 * time.Second
	}
	opts.Name = names.SanitizeName(opts.Name)
	if opts.Name == "" {
		return fmt.Errorf("invalid detached name")
	}

	registry, err := Load()
	if err != nil {
		return err
	}
	if entry, _, ok := registry.state.Find(opts.Name); ok && process.ProcessAlive(entry.PID) {
		if !opts.Replace {
			return fmt.Errorf("%q is already running as pid %d (use --replace or --name)", opts.Name, entry.PID)
		}
		fmt.Fprintf(stdout, "flareduct: replacing %s (pid %d)\n", entry.Name, entry.PID)
		if err := process.TerminatePID(entry.PID, 5*time.Second); err != nil {
			return err
		}
	}
	registry.state.RemoveName(opts.Name)

	result, err := provision.OwnedTunnel(s, stdout, stderr)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(paths.LogsDirPath(), 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(paths.LogsDirPath(), fmt.Sprintf("%s-%s.log", opts.Name, time.Now().Format("20060102-150405")))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}

	cmd := exec.Command(s.Command[0], s.Command[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		provision.CleanupBestEffort(s, result, stdout, stderr)
		return err
	}
	pid := cmd.Process.Pid
	_ = logFile.Close()
	if err := cmd.Process.Release(); err != nil {
		return err
	}

	entry := entryFromStart(opts.Name, s, result, pid, logPath)
	registry.state.Upsert(entry)
	if err := registry.Save(); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "flareduct: started %s as pid %d\n", opts.Name, pid)
	if s.Kind == spec.KindQuick && s.URL != "" {
		fmt.Fprintf(stdout, "flareduct: local service %s\n", s.URL)
	}
	if entry.PublicURL != "" {
		if result := readiness.WaitForReady(entry.PublicURL, opts.Wait); result.Ready {
			fmt.Fprintf(stdout, "flareduct: public URL %s\n", entry.PublicURL)
		} else if result.LastError != nil {
			fmt.Fprintf(stderr, "flareduct: public URL not ready within %s: %v\n", opts.Wait, result.LastError)
			fmt.Fprintf(stdout, "flareduct: public URL %s\n", entry.PublicURL)
		} else {
			fmt.Fprintf(stderr, "flareduct: public URL not ready within %s (last status %d)\n", opts.Wait, result.LastStatus)
			fmt.Fprintf(stdout, "flareduct: public URL %s\n", entry.PublicURL)
		}
		fmt.Fprintf(stdout, "flareduct: logs %s\n", logPath)
		return nil
	}

	if s.Kind == spec.KindQuick {
		if publicURL := waitForPublicURL(logPath, pid, opts.Wait); publicURL != "" {
			entry.PublicURL = publicURL
			registry.state.Upsert(entry)
			_ = registry.Save()
			fmt.Fprintf(stdout, "flareduct: public URL %s\n", publicURL)
		} else {
			fmt.Fprintf(stderr, "flareduct: no public URL detected within %s; check logs\n", opts.Wait)
		}
	}
	fmt.Fprintf(stdout, "flareduct: logs %s\n", logPath)
	return nil
}

func entryFromStart(name string, s spec.Spec, result provision.Result, pid int, logPath string) state.Entry {
	return state.Entry{
		Name:          name,
		Target:        s.Target,
		Kind:          s.Kind,
		URL:           s.URL,
		Hostname:      s.Hostname,
		TunnelName:    s.TunnelName,
		PublicURL:     s.PublicURL,
		AutoCleanup:   s.AutoCleanup,
		CreatedTunnel: result.CreatedTunnel,
		Zone:          s.Zone,
		ZoneID:        s.ZoneID,
		PID:           pid,
		StartedAt:     time.Now(),
		Command:       s.Command,
		LogPath:       logPath,
	}
}

func waitForPublicURL(logPath string, pid int, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(logPath)
		if err == nil {
			if publicURL := strutil.ExtractFirstPublicURLFromBytes(data); publicURL != "" {
				return publicURL
			}
		}
		if !process.ProcessAlive(pid) {
			return ""
		}
		time.Sleep(250 * time.Millisecond)
	}
	return ""
}
