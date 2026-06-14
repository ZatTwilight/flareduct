package paths

import (
	"os"
	"path/filepath"
	"strings"
)

func HomeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}

func ExpandPath(path string) string {
	if path == "" {
		return path
	}
	path = os.ExpandEnv(path)
	if path == "~" {
		return HomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(HomeDir(), path[2:])
	}
	return path
}

func DefaultConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		if userConfigDir, err := os.UserConfigDir(); err == nil && userConfigDir != "" {
			base = userConfigDir
		} else {
			base = filepath.Join(HomeDir(), ".config")
		}
	}
	return filepath.Join(base, "flareduct", "config.yaml")
}

func DefaultStateDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(HomeDir(), ".local", "state")
	}
	return filepath.Join(base, "flareduct")
}

func StateFilePath() string {
	return filepath.Join(DefaultStateDir(), "tunnels.json")
}

func LogsDirPath() string {
	return filepath.Join(DefaultStateDir(), "logs")
}
