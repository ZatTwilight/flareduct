package main

import (
	"fmt"
	"io"
	"os"
)

const version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	globals, rest, err := parseLeadingGlobals(args)
	if err != nil {
		fmt.Fprintf(stderr, "flareduct: %v\n", err)
		return 2
	}
	if globals.Help || len(rest) == 0 {
		printHelp(stdout)
		return 0
	}

	cmd := rest[0]
	cmdArgs := rest[1:]
	var runErr error
	switch cmd {
	case "up", "run":
		runErr = doUp(cmdArgs, globals, stdout, stderr)
	case "list", "ls", "ps":
		runErr = doList(cmdArgs, stdout)
	case "down", "stop":
		runErr = doDown(cmdArgs, stdout, stderr)
	case "logs", "log":
		runErr = doLogs(cmdArgs, stdout)
	case "login":
		runErr = doLogin(cmdArgs, globals, stdout, stderr)
	case "token":
		runErr = doToken(cmdArgs, stdout, stderr)
	case "doctor":
		runErr = doDoctor(cmdArgs, globals, stdout)
	case "config":
		runErr = doConfig(cmdArgs, globals, stdout)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return 0
	case "help", "--help", "-h":
		printHelp(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "flareduct: unknown command %q\n\n", cmd)
		printHelp(stderr)
		return 2
	}
	if runErr != nil {
		fmt.Fprintf(stderr, "flareduct: %v\n", runErr)
		return 1
	}
	return 0
}
