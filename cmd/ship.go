package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"flareduct/internal/names"
	"flareduct/internal/strutil"
	"flareduct/internal/wordslug"
)

type shipOptions struct {
	Help     bool
	Project  string
	Branch   string
	Wrangler string
	Verbose  bool
}

func runShip(args []string, stdout, stderr io.Writer) error {
	opts, target, err := parseShipArgs(args)
	if err != nil {
		return err
	}
	if opts.Help {
		printShipHelp(stdout)
		return nil
	}
	if target == "" {
		printShipHelp(stderr)
		return fmt.Errorf("missing path")
	}
	if opts.Wrangler == "" {
		opts.Wrangler = "wrangler"
	}
	wranglerPath, err := resolveToolPath(opts.Wrangler, "wrangler")
	if err != nil {
		return err
	}

	deployDir, cleanup, err := prepareShipTarget(target)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}

	project := opts.Project
	if project == "" {
		project = wordslug.WordSlug(2)
	}
	project = names.SanitizeName(project)
	if project == "" {
		return fmt.Errorf("invalid project name")
	}
	branch := opts.Branch
	if branch == "" {
		branch = "main"
	}

	cmdArgs := []string{"pages", "deploy", deployDir, "--project-name", project, "--branch", branch, "--commit-dirty=true"}
	fmt.Fprintf(stdout, "flareduct: shipping %s to Cloudflare Pages project %s\n", target, project)

	exists, err := pagesProjectExists(wranglerPath, project, stdout, opts.Verbose)
	if err != nil {
		return err
	}
	if !exists {
		if err := createPagesProject(wranglerPath, project, branch, stdout, stderr, opts.Verbose); err != nil {
			return err
		}
	}

	out, err := runWrangler(wranglerPath, cmdArgs, stdout, stderr, opts.Verbose)
	if err != nil && looksLikeMissingPagesProject(out) {
		// Fallback for stale Wrangler/API project list results.
		if err := createPagesProject(wranglerPath, project, branch, stdout, stderr, opts.Verbose); err != nil {
			return err
		}
		out, err = runWrangler(wranglerPath, cmdArgs, stdout, stderr, opts.Verbose)
	}
	if err != nil {
		return fmt.Errorf("wrangler pages deploy failed: %w", err)
	}

	url := "https://" + project + ".pages.dev/"
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "╭─ flareduct shipped ───────────────────────────────────")
	fmt.Fprintf(stdout, "│ %s\n", url)
	fmt.Fprintln(stdout, "╰────────────────────────────────────────────────────────")
	return nil
}

func runWrangler(wranglerPath string, args []string, stdout, stderr io.Writer, verbose bool) (string, error) {
	if verbose {
		fmt.Fprintf(stdout, "flareduct: running %s\n", strutil.ShellQuote(append([]string{wranglerPath}, args...)))
	}
	cmd := exec.Command(wranglerPath, args...)
	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(stdout, &out)
	cmd.Stderr = io.MultiWriter(stderr, &out)
	err := cmd.Run()
	return out.String(), err
}

func runWranglerCapture(wranglerPath string, args []string, stdout io.Writer, verbose bool) (string, error) {
	if verbose {
		fmt.Fprintf(stdout, "flareduct: running %s\n", strutil.ShellQuote(append([]string{wranglerPath}, args...)))
	}
	cmd := exec.Command(wranglerPath, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func pagesProjectExists(wranglerPath, project string, stdout io.Writer, verbose bool) (bool, error) {
	out, err := runWranglerCapture(wranglerPath, []string{"pages", "project", "list", "--json"}, stdout, verbose)
	if err != nil {
		return false, fmt.Errorf("wrangler pages project list failed: %w", err)
	}
	var projects []map[string]any
	if err := json.Unmarshal([]byte(out), &projects); err != nil {
		return false, fmt.Errorf("parse wrangler pages project list: %w", err)
	}
	for _, p := range projects {
		if name, _ := p["Project Name"].(string); name == project {
			return true, nil
		}
		if name, _ := p["name"].(string); name == project {
			return true, nil
		}
	}
	return false, nil
}

func createPagesProject(wranglerPath, project, branch string, stdout, stderr io.Writer, verbose bool) error {
	fmt.Fprintf(stdout, "flareduct: creating Cloudflare Pages project %s\n", project)
	createArgs := []string{"pages", "project", "create", project, "--production-branch", branch}
	if _, err := runWrangler(wranglerPath, createArgs, stdout, stderr, verbose); err != nil {
		return fmt.Errorf("wrangler pages project create failed: %w", err)
	}
	return nil
}

func looksLikeMissingPagesProject(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "project not found") || strings.Contains(lower, "code: 8000007")
}

func parseShipArgs(args []string) (shipOptions, string, error) {
	var opts shipOptions
	var target string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			opts.Help = true
		case a == "--project" || a == "--project-name" || a == "--name":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("%s needs a name", a)
			}
			i++
			opts.Project = args[i]
		case strings.HasPrefix(a, "--project="):
			opts.Project = strings.TrimPrefix(a, "--project=")
		case strings.HasPrefix(a, "--project-name="):
			opts.Project = strings.TrimPrefix(a, "--project-name=")
		case strings.HasPrefix(a, "--name="):
			opts.Project = strings.TrimPrefix(a, "--name=")
		case a == "--branch":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--branch needs a name")
			}
			i++
			opts.Branch = args[i]
		case strings.HasPrefix(a, "--branch="):
			opts.Branch = strings.TrimPrefix(a, "--branch=")
		case a == "--wrangler":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--wrangler needs a path")
			}
			i++
			opts.Wrangler = args[i]
		case strings.HasPrefix(a, "--wrangler="):
			opts.Wrangler = strings.TrimPrefix(a, "--wrangler=")
		case a == "--verbose":
			opts.Verbose = true
		case strings.HasPrefix(a, "-"):
			return opts, target, fmt.Errorf("unknown ship flag %q", a)
		default:
			if target != "" {
				return opts, target, fmt.Errorf("too many arguments")
			}
			target = a
		}
	}
	return opts, target, nil
}

func prepareShipTarget(target string) (string, func(), error) {
	path := filepath.Clean(target)
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		return path, nil, nil
	}
	tmp, err := os.MkdirTemp("", "flareduct-ship-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	data, err := os.ReadFile(path)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	name := filepath.Base(path)
	if strings.EqualFold(filepath.Ext(name), ".html") || strings.EqualFold(filepath.Ext(name), ".htm") {
		name = "index.html"
	}
	if err := os.WriteFile(filepath.Join(tmp, name), data, info.Mode().Perm()); err != nil {
		cleanup()
		return "", nil, err
	}
	return tmp, cleanup, nil
}

func resolveToolPath(tool, label string) (string, error) {
	path, err := exec.LookPath(tool)
	if err != nil && strings.Contains(tool, string(os.PathSeparator)) {
		if info, statErr := os.Stat(tool); statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			path = tool
			err = nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("%s not found at %q: %w (install %s and run `%s login` first)", label, tool, err, label, label)
	}
	return path, nil
}
