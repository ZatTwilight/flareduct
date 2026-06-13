package main

import (
	"fmt"
	"net/http"
	"time"
)

const (
	publicURLReadinessTimeout     = 90 * time.Second
	publicURLReadinessPoll        = 1 * time.Second
	publicURLReadinessSuccesses   = 3
	publicURLReadinessHTTPTimeout = 8 * time.Second
)

type publicURLReadinessResult struct {
	Ready       bool
	LastStatus  int
	LastError   error
	Attempts    int
	Consecutive int
}

func WaitForPublicURLReady(publicURL string, timeout time.Duration) publicURLReadinessResult {
	return waitForPublicURLReady(publicURL, timeout, publicURLReadinessSuccesses, publicURLReadinessPoll, http.DefaultClient)
}

func waitForPublicURLReady(publicURL string, timeout time.Duration, successes int, poll time.Duration, client *http.Client) publicURLReadinessResult {
	if publicURL == "" {
		return publicURLReadinessResult{LastError: fmt.Errorf("empty public URL")}
	}
	if timeout <= 0 {
		timeout = publicURLReadinessTimeout
	}
	if successes <= 0 {
		successes = 1
	}
	if poll <= 0 {
		poll = publicURLReadinessPoll
	}
	if client == nil {
		client = http.DefaultClient
	}

	deadline := time.Now().Add(timeout)
	var result publicURLReadinessResult
	for time.Now().Before(deadline) {
		status, err := probePublicURL(client, publicURL)
		result.Attempts++
		result.LastStatus = status
		result.LastError = err
		if err == nil && publicURLStatusReady(status) {
			result.Consecutive++
			if result.Consecutive >= successes {
				result.Ready = true
				return result
			}
		} else {
			result.Consecutive = 0
		}
		time.Sleep(poll)
	}
	return result
}

func probePublicURL(client *http.Client, publicURL string) (int, error) {
	req, err := http.NewRequest(http.MethodHead, publicURL, nil)
	if err != nil {
		return 0, err
	}
	probeClient := *client
	if probeClient.Timeout == 0 {
		probeClient.Timeout = publicURLReadinessHTTPTimeout
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func publicURLStatusReady(status int) bool {
	// 525/526/527/530 are Cloudflare edge/origin readiness failures. Treat any
	// non-Cloudflare-5xx response as ready: an app-level 404 still proves the
	// hostname, certificate, tunnel route, and origin handshake are working.
	if status == 525 || status == 526 || status == 527 || status == 530 {
		return false
	}
	return status > 0 && status < 500
}
