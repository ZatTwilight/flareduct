package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"text/tabwriter"
	"time"
)

type DetachOptions struct {
	Name    string
	Replace bool
	Wait    time.Duration
}

type DetachedRegistry struct {
	state State
}

func LoadDetachedRegistry() (*DetachedRegistry, error) {
	state, err := LoadState()
	if err != nil {
		return nil, err
	}
	return &DetachedRegistry{state: state}, nil
}

func (r *DetachedRegistry) Save() error {
	return SaveState(r.state)
}

func ListDetachedTunnels(stdout io.Writer) error {
	registry, err := LoadDetachedRegistry()
	if err != nil {
		return err
	}
	return registry.List(stdout)
}

func (r *DetachedRegistry) List(stdout io.Writer) error {
	if len(r.state.Entries) == 0 {
		fmt.Fprintln(stdout, "No detached tunnels.")
		return nil
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPID\tSTATUS\tKIND\tTARGET\tPUBLIC URL\tLOG")
	for _, entry := range r.state.Entries {
		status := "stopped"
		if ProcessAlive(entry.PID) {
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

func StopDetachedTunnels(key string, all bool, stdout, stderr io.Writer) error {
	registry, err := LoadDetachedRegistry()
	if err != nil {
		return err
	}
	return registry.Stop(key, all, stdout, stderr)
}

func (r *DetachedRegistry) Stop(key string, all bool, stdout, stderr io.Writer) error {
	if len(r.state.Entries) == 0 {
		fmt.Fprintln(stdout, "No detached tunnels.")
		return nil
	}

	if all {
		for _, entry := range r.state.Entries {
			if ProcessAlive(entry.PID) {
				fmt.Fprintf(stdout, "Stopping %s (pid %d)\n", entry.Name, entry.PID)
				if err := TerminatePID(entry.PID, 5*time.Second); err != nil {
					return err
				}
			}
			cleanupOwnedTunnelBestEffort(specFromStateEntry(entry), ProvisionResult{CreatedTunnel: entry.CreatedTunnel}, stdout, stderr)
		}
		r.state.Entries = nil
		return r.Save()
	}

	entry, idx, ok := r.find(key)
	if !ok {
		return fmt.Errorf("no detached tunnel named/pid %q", key)
	}
	if ProcessAlive(entry.PID) {
		fmt.Fprintf(stdout, "Stopping %s (pid %d)\n", entry.Name, entry.PID)
		if err := TerminatePID(entry.PID, 5*time.Second); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(stdout, "%s was already stopped\n", entry.Name)
	}
	cleanupOwnedTunnelBestEffort(specFromStateEntry(entry), ProvisionResult{CreatedTunnel: entry.CreatedTunnel}, stdout, stderr)
	r.removeIndex(idx)
	return r.Save()
}

func ShowDetachedLogs(key string, lines int, follow bool, stdout io.Writer) error {
	registry, err := LoadDetachedRegistry()
	if err != nil {
		return err
	}
	return registry.Logs(key, lines, follow, stdout)
}

func (r *DetachedRegistry) Logs(key string, lines int, follow bool, stdout io.Writer) error {
	entry, _, ok := r.find(key)
	if !ok {
		return fmt.Errorf("no detached tunnel named/pid %q", key)
	}
	if entry.LogPath == "" {
		return fmt.Errorf("no log path stored for %q", entry.Name)
	}
	if err := PrintLinesTail(entry.LogPath, lines, stdout); err != nil {
		return err
	}
	if follow {
		return FollowFile(entry.LogPath, stdout)
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

	registry, err := LoadDetachedRegistry()
	if err != nil {
		return err
	}
	if entry, _, ok := registry.find(opts.Name); ok && ProcessAlive(entry.PID) {
		if !opts.Replace {
			return fmt.Errorf("%q is already running as pid %d (use --replace or --name)", opts.Name, entry.PID)
		}
		fmt.Fprintf(stdout, "flareduct: replacing %s (pid %d)\n", entry.Name, entry.PID)
		if err := TerminatePID(entry.PID, 5*time.Second); err != nil {
			return err
		}
	}
	registry.removeName(opts.Name)

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

	entry := stateEntryFromDetachedStart(opts.Name, spec, provision, pid, logPath)
	registry.upsert(entry)
	if err := registry.Save(); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "flareduct: started %s as pid %d\n", opts.Name, pid)
	if spec.Kind == KindQuick && spec.URL != "" {
		fmt.Fprintf(stdout, "flareduct: local service %s\n", spec.URL)
	}
	if entry.PublicURL != "" {
		if result := WaitForPublicURLReady(entry.PublicURL, opts.Wait); result.Ready {
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

	if spec.Kind == KindQuick {
		if publicURL := WaitForPublicURL(logPath, pid, opts.Wait); publicURL != "" {
			entry.PublicURL = publicURL
			registry.upsert(entry)
			_ = registry.Save()
			fmt.Fprintf(stdout, "flareduct: public URL %s\n", publicURL)
		} else {
			fmt.Fprintf(stderr, "flareduct: no public URL detected within %s; check logs\n", opts.Wait)
		}
	}
	fmt.Fprintf(stdout, "flareduct: logs %s\n", logPath)
	return nil
}

func stateEntryFromDetachedStart(name string, spec TunnelSpec, provision ProvisionResult, pid int, logPath string) StateEntry {
	return StateEntry{
		Name:          name,
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
}

func (r *DetachedRegistry) removeName(name string) bool {
	removed := false
	kept := r.state.Entries[:0]
	for _, entry := range r.state.Entries {
		if entry.Name == name {
			removed = true
			continue
		}
		kept = append(kept, entry)
	}
	r.state.Entries = kept
	return removed
}

func (r *DetachedRegistry) upsert(entry StateEntry) {
	for i := range r.state.Entries {
		if r.state.Entries[i].Name == entry.Name {
			r.state.Entries[i] = entry
			return
		}
	}
	r.state.Entries = append(r.state.Entries, entry)
}

func (r *DetachedRegistry) find(key string) (StateEntry, int, bool) {
	for i, entry := range r.state.Entries {
		if entry.Name == key {
			return entry, i, true
		}
	}
	if pid, err := strconv.Atoi(key); err == nil {
		for i, entry := range r.state.Entries {
			if entry.PID == pid {
				return entry, i, true
			}
		}
	}
	return StateEntry{}, -1, false
}

func (r *DetachedRegistry) removeIndex(idx int) {
	if idx < 0 || idx >= len(r.state.Entries) {
		return
	}
	copy(r.state.Entries[idx:], r.state.Entries[idx+1:])
	r.state.Entries = r.state.Entries[:len(r.state.Entries)-1]
}

func (s *State) RemoveName(name string) bool {
	registry := DetachedRegistry{state: *s}
	removed := registry.removeName(name)
	*s = registry.state
	return removed
}

func (s *State) Upsert(entry StateEntry) {
	registry := DetachedRegistry{state: *s}
	registry.upsert(entry)
	*s = registry.state
}

func (s State) Find(key string) (StateEntry, int, bool) {
	registry := DetachedRegistry{state: s}
	return registry.find(key)
}

func (s *State) RemoveIndex(idx int) {
	registry := DetachedRegistry{state: *s}
	registry.removeIndex(idx)
	*s = registry.state
}

func (s *State) PruneStopped() int {
	removed := 0
	kept := s.Entries[:0]
	for _, entry := range s.Entries {
		if ProcessAlive(entry.PID) {
			kept = append(kept, entry)
		} else {
			removed++
		}
	}
	s.Entries = kept
	return removed
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
