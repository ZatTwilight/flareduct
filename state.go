package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type State struct {
	Entries []StateEntry `json:"entries"`
}

type StateEntry struct {
	Name          string     `json:"name"`
	Target        string     `json:"target"`
	Kind          TunnelKind `json:"kind"`
	URL           string     `json:"url,omitempty"`
	Hostname      string     `json:"hostname,omitempty"`
	TunnelName    string     `json:"tunnel_name,omitempty"`
	PublicURL     string     `json:"public_url,omitempty"`
	AutoCleanup   bool       `json:"auto_cleanup,omitempty"`
	CreatedTunnel bool       `json:"created_tunnel,omitempty"`
	Zone          string     `json:"zone,omitempty"`
	ZoneID        string     `json:"zone_id,omitempty"`
	PID           int        `json:"pid"`
	StartedAt     time.Time  `json:"started_at"`
	Command       []string   `json:"command"`
	LogPath       string     `json:"log_path"`
}

func LoadState() (State, error) {
	var state State
	path := StateFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if len(data) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("parse state %s: %w", path, err)
	}
	return state, nil
}

func SaveState(state State) error {
	path := StateFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *State) RemoveName(name string) bool {
	removed := false
	kept := s.Entries[:0]
	for _, entry := range s.Entries {
		if entry.Name == name {
			removed = true
			continue
		}
		kept = append(kept, entry)
	}
	s.Entries = kept
	return removed
}

func (s *State) Upsert(entry StateEntry) {
	for i := range s.Entries {
		if s.Entries[i].Name == entry.Name {
			s.Entries[i] = entry
			return
		}
	}
	s.Entries = append(s.Entries, entry)
}

func (s State) Find(key string) (StateEntry, int, bool) {
	for i, entry := range s.Entries {
		if entry.Name == key {
			return entry, i, true
		}
	}
	if pid, err := strconv.Atoi(key); err == nil {
		for i, entry := range s.Entries {
			if entry.PID == pid {
				return entry, i, true
			}
		}
	}
	return StateEntry{}, -1, false
}

func (s *State) RemoveIndex(idx int) {
	if idx < 0 || idx >= len(s.Entries) {
		return
	}
	copy(s.Entries[idx:], s.Entries[idx+1:])
	s.Entries = s.Entries[:len(s.Entries)-1]
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
