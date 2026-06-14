package provision

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"flareduct/internal/cloudflare"
	"flareduct/internal/spec"
	"flareduct/internal/state"
	"flareduct/internal/strutil"
	"flareduct/internal/token"
)

type Result struct {
	CreatedTunnel bool
}

func OwnedTunnel(spec spec.Spec, stdout, stderr io.Writer) (Result, error) {
	var result Result
	if spec.Hostname == "" || spec.TunnelName == "" {
		return result, nil
	}
	if len(spec.Command) == 0 {
		return result, fmt.Errorf("empty command")
	}
	cloudflared := spec.Command[0]

	exists, err := tunnelExists(cloudflared, spec.TunnelName)
	if err != nil {
		return result, err
	}
	if exists {
		fmt.Fprintf(stdout, "flareduct: tunnel %s already exists; auto-cleanup will not delete existing resources\n", spec.TunnelName)
	} else {
		createCmd := []string{cloudflared, "tunnel", "create", spec.TunnelName}
		if err := runCommand("creating tunnel "+spec.TunnelName, createCmd, stdout, stderr, spec.Verbose); err != nil {
			return result, err
		}
		result.CreatedTunnel = true
	}

	routeCmd := []string{cloudflared, "tunnel", "route", "dns"}
	if spec.OverwriteDNS {
		routeCmd = append(routeCmd, "--overwrite-dns")
	}
	routeCmd = append(routeCmd, spec.TunnelName, spec.Hostname)
	if err := runCommand("routing DNS "+spec.Hostname, routeCmd, stdout, stderr, spec.Verbose); err != nil {
		if result.CreatedTunnel {
			CleanupBestEffort(spec, result, stdout, stderr)
		}
		return result, err
	}
	return result, nil
}

func tunnelExists(cloudflared, tunnelName string) (bool, error) {
	cmd := exec.Command(cloudflared, "tunnel", "info", tunnelName)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, err
}

func runCommand(label string, args []string, stdout, stderr io.Writer, verbose bool) error {
	fmt.Fprintf(stdout, "flareduct: %s\n", label)
	if verbose {
		fmt.Fprintf(stdout, "flareduct: running %s\n", strutil.ShellQuote(args))
	}
	cmd := exec.Command(args[0], args[1:]...)
	if verbose {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("provisioning failed for %s: %w", strutil.ShellQuote(args), err)
		}
		return nil
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			fmt.Fprint(stderr, string(out))
		}
		return fmt.Errorf("provisioning failed for %s: %w", strutil.ShellQuote(args), err)
	}
	return nil
}

func CleanupBestEffort(spec spec.Spec, provision Result, stdout, stderr io.Writer) {
	if err := Cleanup(spec, provision, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "flareduct: cleanup warning: %v\n", err)
	}
}

func SpecFromEntry(entry state.Entry) spec.Spec {
	return spec.Spec{
		Name:        entry.Name,
		Target:      entry.Target,
		Kind:        entry.Kind,
		URL:         entry.URL,
		Hostname:    entry.Hostname,
		TunnelName:  entry.TunnelName,
		PublicURL:   entry.PublicURL,
		AutoCleanup: entry.AutoCleanup,
		Zone:        entry.Zone,
		ZoneID:      entry.ZoneID,
		Command:     entry.Command,
	}
}

func Cleanup(spec spec.Spec, provision Result, stdout, stderr io.Writer) error {
	if spec.Hostname == "" || spec.TunnelName == "" || !spec.AutoCleanup || !provision.CreatedTunnel {
		return nil
	}
	if len(spec.Command) == 0 {
		return fmt.Errorf("empty command")
	}
	cloudflared := spec.Command[0]

	fmt.Fprintf(stdout, "flareduct: cleaning up %s\n", spec.PublicURL)
	if err := cleanupDNSRecord(spec, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "flareduct: DNS cleanup warning: %v\n", err)
	}

	deleteCmd := []string{cloudflared, "tunnel", "delete", "-f", spec.TunnelName}
	fmt.Fprintf(stdout, "flareduct: cleanup %s\n", strutil.ShellQuote(deleteCmd))
	cmd := execCommand(deleteCmd, stdout, stderr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("delete tunnel %s: %w", spec.TunnelName, err)
	}
	return nil
}

func cleanupDNSRecord(spec spec.Spec, stdout, stderr io.Writer) error {
	tok, source, ok, err := token.LoadCloudflareAPIToken()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no Cloudflare API token configured; run `flareduct token set` or set CLOUDFLARE_API_TOKEN; DNS record %s may remain", spec.Hostname)
	}
	fmt.Fprintf(stdout, "flareduct: using Cloudflare API token from %s for DNS cleanup\n", source)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := cloudflare.NewClient(tok, nil)

	zoneID := spec.ZoneID
	if zoneID == "" {
		var err error
		zoneID, err = client.FindZoneID(ctx, spec.Hostname, spec.Zone)
		if err != nil {
			return err
		}
	}

	records, err := client.FindDNSRecords(ctx, zoneID, spec.Hostname)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		fmt.Fprintf(stdout, "flareduct: DNS record %s was already gone\n", spec.Hostname)
		return nil
	}
	for _, record := range records {
		fmt.Fprintf(stdout, "flareduct: deleting DNS record %s (%s)\n", record.Name, record.Type)
		if err := client.DeleteDNSRecord(ctx, zoneID, record.ID); err != nil {
			return err
		}
	}
	return nil
}

type commandRunner interface {
	Run() error
}

var execCommand = func(args []string, stdout, stderr io.Writer) commandRunner {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd
}
