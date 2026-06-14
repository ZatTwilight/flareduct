package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"flareduct/internal/token"
)

func runToken(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		printTokenHelp(stdout)
		return nil
	}
	switch args[0] {
	case "set":
		if len(args) > 2 {
			return fmt.Errorf("usage: flareduct token set [TOKEN]")
		}
		tok := ""
		var err error
		if len(args) == 2 {
			tok = strings.TrimSpace(args[1])
			fmt.Fprintln(stderr, "flareduct: warning: passing tokens as arguments can leave them in shell history; prefer `flareduct token set` and paste when prompted")
		} else {
			tok, err = token.ReadTokenInteractively(os.Stdin, stdout)
			if err != nil {
				return err
			}
		}
		path, err := token.SaveCloudflareAPIToken(tok)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "saved Cloudflare API token to %s\n", path)
		fmt.Fprintln(stdout, "token file permissions set to 0600")
		return nil
	case "status":
		_, source, ok, err := token.LoadCloudflareAPIToken()
		if err != nil {
			return err
		}
		if ok {
			fmt.Fprintf(stdout, "Cloudflare API token configured from %s\n", source)
		} else {
			fmt.Fprintf(stdout, "No Cloudflare API token configured. Token file path: %s\n", source)
		}
		return nil
	case "path":
		fmt.Fprintln(stdout, token.DefaultTokenPath())
		return nil
	case "remove", "rm":
		path, err := token.RemoveCloudflareAPIToken()
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "removed %s\n", path)
		return nil
	default:
		return fmt.Errorf("unknown token subcommand %q", args[0])
	}
}
