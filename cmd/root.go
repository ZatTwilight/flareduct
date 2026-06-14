package cmd

import (
	"fmt"
	"io"

	"flareduct/internal/cli"
)

var version = "dev"

func Run(args []string, stdout, stderr io.Writer) int {
	globals, rest, err := cli.ParseLeadingGlobals(args)
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
		runErr = runUp(cmdArgs, globals, stdout, stderr)
	case "list", "ls", "ps":
		runErr = runList(cmdArgs, stdout)
	case "down", "stop":
		runErr = runDown(cmdArgs, stdout, stderr)
	case "logs", "log":
		runErr = runLogs(cmdArgs, stdout)
	case "login":
		runErr = runLogin(cmdArgs, globals, stdout, stderr)
	case "token":
		runErr = runToken(cmdArgs, stdout, stderr)
	case "doctor":
		runErr = runDoctor(cmdArgs, globals, stdout)
	case "config":
		runErr = runConfig(cmdArgs, globals, stdout)
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
