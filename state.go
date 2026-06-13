package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
