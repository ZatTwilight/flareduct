package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"flareduct/internal/cli"
	"flareduct/internal/cloudflare"
	"flareduct/internal/config"
	"flareduct/internal/names"
	"flareduct/internal/strutil"
	"flareduct/internal/token"
	"flareduct/internal/wordslug"
)

type shipOptions struct {
	Help      bool
	Project   string
	Branch    string
	Wrangler  string
	Hostname  string
	Subdomain string
	Domain    string
	AccountID string
	PagesDev  bool
	Verbose   bool
}

func runShip(args []string, globals cli.GlobalOptions, stdout, stderr io.Writer) error {
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
	cfg, _, _, err := config.LoadConfig(globals.ConfigPath)
	if err != nil {
		return err
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
	wranglerWorkDir, err := os.MkdirTemp("", "flareduct-wrangler-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(wranglerWorkDir)

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

	exists, err := pagesProjectExists(wranglerPath, wranglerWorkDir, project, stdout, opts.Verbose)
	if err != nil {
		return err
	}
	if !exists {
		if err := createPagesProject(wranglerPath, wranglerWorkDir, project, branch, stdout, stderr, opts.Verbose); err != nil {
			return err
		}
	}

	out, err := runWrangler(wranglerPath, wranglerWorkDir, cmdArgs, stdout, stderr, opts.Verbose)
	if err != nil && looksLikeMissingPagesProject(out) {
		// Fallback for stale Wrangler/API project list results.
		if err := createPagesProject(wranglerPath, wranglerWorkDir, project, branch, stdout, stderr, opts.Verbose); err != nil {
			return err
		}
		out, err = runWrangler(wranglerPath, wranglerWorkDir, cmdArgs, stdout, stderr, opts.Verbose)
	}
	if err != nil {
		return fmt.Errorf("wrangler pages deploy failed: %w", err)
	}

	pagesURL := "https://" + project + ".pages.dev/"
	publicURL := pagesURL
	if hostname, ok, err := resolveShipHostname(opts, cfg, project); err != nil {
		return err
	} else if ok {
		if err := attachPagesDomain(hostname, project, opts.AccountID, cfg, wranglerPath, wranglerWorkDir, stdout, stderr, opts.Verbose); err != nil {
			fmt.Fprintf(stderr, "flareduct: custom domain warning: %v\n", err)
		} else {
			publicURL = "https://" + hostname + "/"
		}
	}

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "╭─ flareduct shipped ───────────────────────────────────")
	fmt.Fprintf(stdout, "│ %s\n", publicURL)
	if publicURL != pagesURL {
		fmt.Fprintf(stdout, "│ PAGES   %s\n", pagesURL)
	}
	fmt.Fprintln(stdout, "╰────────────────────────────────────────────────────────")
	return nil
}

func runWrangler(wranglerPath, cwd string, args []string, stdout, stderr io.Writer, verbose bool) (string, error) {
	if verbose {
		fmt.Fprintf(stdout, "flareduct: running %s\n", strutil.ShellQuote(append([]string{wranglerPath}, args...)))
	}
	cmd := exec.Command(wranglerPath, args...)
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(stdout, &out)
	cmd.Stderr = io.MultiWriter(stderr, &out)
	err := cmd.Run()
	return out.String(), err
}

func runWranglerCapture(wranglerPath, cwd string, args []string, stdout io.Writer, verbose bool) (string, error) {
	if verbose {
		fmt.Fprintf(stdout, "flareduct: running %s\n", strutil.ShellQuote(append([]string{wranglerPath}, args...)))
	}
	cmd := exec.Command(wranglerPath, args...)
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func pagesProjectExists(wranglerPath, cwd, project string, stdout io.Writer, verbose bool) (bool, error) {
	out, err := runWranglerCapture(wranglerPath, cwd, []string{"pages", "project", "list", "--json"}, stdout, verbose)
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

func createPagesProject(wranglerPath, cwd, project, branch string, stdout, stderr io.Writer, verbose bool) error {
	fmt.Fprintf(stdout, "flareduct: creating Cloudflare Pages project %s\n", project)
	createArgs := []string{"pages", "project", "create", project, "--production-branch", branch}
	if _, err := runWrangler(wranglerPath, cwd, createArgs, stdout, stderr, verbose); err != nil {
		return fmt.Errorf("wrangler pages project create failed: %w", err)
	}
	return nil
}

func looksLikeMissingPagesProject(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "project not found") || strings.Contains(lower, "code: 8000007")
}

func resolveShipHostname(opts shipOptions, cfg config.Config, project string) (string, bool, error) {
	if opts.PagesDev {
		return "", false, nil
	}
	if opts.Hostname != "" && opts.Subdomain != "" {
		return "", false, fmt.Errorf("use either --hostname or --subdomain, not both")
	}
	if opts.Hostname != "" {
		hostname, err := names.NormalizeHostname(opts.Hostname)
		return hostname, err == nil, err
	}
	domain := strings.TrimSpace(opts.Domain)
	if domain == "" {
		domain = strings.TrimSpace(cfg.Pages.Domain)
	}
	if domain == "" {
		domain = strings.TrimSpace(cfg.Public.Domain)
	}
	if domain == "" {
		return "", false, nil
	}
	domain, err := names.NormalizeHostname(domain)
	if err != nil {
		return "", false, fmt.Errorf("invalid pages domain: %w", err)
	}
	subdomain := opts.Subdomain
	if subdomain == "" {
		subdomain = project
	}
	subdomain, err = names.NormalizeSubdomain(subdomain)
	if err != nil {
		return "", false, err
	}
	return subdomain + "." + domain, true, nil
}

func attachPagesDomain(hostname, project, accountOverride string, cfg config.Config, wranglerPath, cwd string, stdout, stderr io.Writer, verbose bool) error {
	apiToken, source, ok, err := token.LoadCloudflareAPIToken()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no Cloudflare API token configured; run `flareduct token set` to attach %s", hostname)
	}
	accountID := strings.TrimSpace(accountOverride)
	if accountID == "" {
		accountID = strings.TrimSpace(cfg.Pages.AccountID)
	}
	if accountID == "" {
		accountID, err = wranglerAccountID(wranglerPath, cwd, stdout, verbose)
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "flareduct: attaching custom domain %s using token from %s\n", hostname, source)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := cloudflare.NewClient(apiToken, nil)
	if err := client.AddPagesDomain(ctx, accountID, project, hostname); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already") {
		return err
	}
	return nil
}

func wranglerAccountID(wranglerPath, cwd string, stdout io.Writer, verbose bool) (string, error) {
	out, err := runWranglerCapture(wranglerPath, cwd, []string{"whoami", "--json"}, stdout, verbose)
	if err != nil {
		return "", fmt.Errorf("wrangler whoami failed: %w", err)
	}
	var info struct {
		Accounts []struct {
			ID string `json:"id"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return "", fmt.Errorf("parse wrangler whoami: %w", err)
	}
	if len(info.Accounts) == 0 || strings.TrimSpace(info.Accounts[0].ID) == "" {
		return "", fmt.Errorf("could not determine Cloudflare account id; set pages.account_id or pass --account-id")
	}
	return strings.TrimSpace(info.Accounts[0].ID), nil
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
		case a == "--hostname":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--hostname needs a host")
			}
			i++
			opts.Hostname = args[i]
		case strings.HasPrefix(a, "--hostname="):
			opts.Hostname = strings.TrimPrefix(a, "--hostname=")
		case a == "--subdomain":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--subdomain needs a name")
			}
			i++
			opts.Subdomain = args[i]
		case strings.HasPrefix(a, "--subdomain="):
			opts.Subdomain = strings.TrimPrefix(a, "--subdomain=")
		case a == "--domain":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--domain needs a domain")
			}
			i++
			opts.Domain = args[i]
		case strings.HasPrefix(a, "--domain="):
			opts.Domain = strings.TrimPrefix(a, "--domain=")
		case a == "--account-id":
			if i+1 >= len(args) {
				return opts, target, fmt.Errorf("--account-id needs an id")
			}
			i++
			opts.AccountID = args[i]
		case strings.HasPrefix(a, "--account-id="):
			opts.AccountID = strings.TrimPrefix(a, "--account-id=")
		case a == "--pages-dev":
			opts.PagesDev = true
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
	path, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return "", nil, err
	}
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
