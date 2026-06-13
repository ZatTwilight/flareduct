package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

func doList(args []string, stdout io.Writer) error {
	if hasHelp(args) {
		printListHelp(stdout)
		return nil
	}
	return ListDetachedTunnels(stdout)
}

func doDown(args []string, stdout, stderr io.Writer) error {
	all := false
	var key string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-h", "--help":
			printDownHelp(stdout)
			return nil
		case "--all", "-a":
			all = true
		default:
			if strings.HasPrefix(a, "-") {
				return fmt.Errorf("unknown down flag %q", a)
			}
			if key != "" {
				return fmt.Errorf("too many arguments")
			}
			key = a
		}
	}
	if !all && key == "" {
		printDownHelp(stdout)
		return fmt.Errorf("missing name or pid")
	}
	return StopDetachedTunnels(key, all, stdout, stderr)
}

func doLogs(args []string, stdout io.Writer) error {
	follow := false
	lines := 80
	var key string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			printLogsHelp(stdout)
			return nil
		case a == "-f" || a == "--follow":
			follow = true
		case a == "-n" || a == "--lines":
			if i+1 >= len(args) {
				return fmt.Errorf("%s needs a number", a)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				return fmt.Errorf("invalid line count %q", args[i])
			}
			lines = n
		case strings.HasPrefix(a, "--lines="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--lines="))
			if err != nil || n < 0 {
				return fmt.Errorf("invalid line count %q", a)
			}
			lines = n
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown logs flag %q", a)
		default:
			if key != "" {
				return fmt.Errorf("too many arguments")
			}
			key = a
		}
	}
	if key == "" {
		printLogsHelp(stdout)
		return fmt.Errorf("missing name or pid")
	}
	return ShowDetachedLogs(key, lines, follow, stdout)
}
