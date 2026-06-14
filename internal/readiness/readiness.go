package readiness

import (
	"fmt"
	"net/http"
	"time"
)

const (
	defaultTimeout     = 90 * time.Second
	defaultPoll        = 1 * time.Second
	defaultSuccesses   = 3
	defaultHTTPTimeout = 8 * time.Second
)

type Result struct {
	Ready       bool
	LastStatus  int
	LastError   error
	Attempts    int
	Consecutive int
}

func WaitForReady(publicURL string, timeout time.Duration) Result {
	return waitForReady(publicURL, timeout, defaultSuccesses, defaultPoll, http.DefaultClient)
}

func waitForReady(publicURL string, timeout time.Duration, successes int, poll time.Duration, client *http.Client) Result {
	if publicURL == "" {
		return Result{LastError: fmt.Errorf("empty public URL")}
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if successes <= 0 {
		successes = 1
	}
	if poll <= 0 {
		poll = defaultPoll
	}
	if client == nil {
		client = http.DefaultClient
	}

	deadline := time.Now().Add(timeout)
	var result Result
	for time.Now().Before(deadline) {
		status, err := probe(client, publicURL)
		result.Attempts++
		result.LastStatus = status
		result.LastError = err
		if err == nil && IsStatusReady(status) {
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

func probe(client *http.Client, publicURL string) (int, error) {
	req, err := http.NewRequest(http.MethodHead, publicURL, nil)
	if err != nil {
		return 0, err
	}
	probeClient := *client
	if probeClient.Timeout == 0 {
		probeClient.Timeout = defaultHTTPTimeout
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func IsStatusReady(status int) bool {
	if status == 525 || status == 526 || status == 527 || status == 530 {
		return false
	}
	return status > 0 && status < 500
}
