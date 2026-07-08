package skycloak

import (
	"io"
	"net/http"
	"strconv"
	"time"
)

// retryTransport retries transient responses (429, 502/503/504, and a 409 that
// the server marks retryable with a Retry-After header), honoring a numeric
// Retry-After header, with bounded exponential backoff. It is request-context
// aware.
type retryTransport struct {
	base       http.RoundTripper
	maxRetries int
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	var resp *http.Response
	var err error
	for attempt := 0; ; attempt++ {
		// Reset the body for replays (oapi-codegen sets GetBody for JSON bodies).
		if attempt > 0 && req.GetBody != nil {
			body, gerr := req.GetBody()
			if gerr != nil {
				return resp, err
			}
			req.Body = body
		}
		resp, err = base.RoundTrip(req)
		if err != nil {
			return resp, err
		}
		if !shouldRetry(resp) || attempt >= t.maxRetries {
			return resp, nil
		}
		wait := backoffDelay(attempt, resp.Header.Get("Retry-After"))
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(wait):
		}
	}
}

// shouldRetry reports whether resp is a transient failure worth replaying.
func shouldRetry(resp *http.Response) bool {
	if retryableStatus(resp.StatusCode) {
		return true
	}
	// A 409 is retried only when the server explicitly marks it retryable with a
	// Retry-After header (e.g. "cluster is updating", which self-resolves once
	// the update finishes). Terminal conflicts such as a name already taken carry
	// no Retry-After and must surface to the user rather than loop.
	return resp.StatusCode == http.StatusConflict && resp.Header.Get("Retry-After") != ""
}

func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}

// backoffDelay returns the wait before the next attempt: the server's numeric
// Retry-After if present, else capped exponential backoff (1s, 2s, 4s, ... ≤30s).
func backoffDelay(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(retryAfter); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}
