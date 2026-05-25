package skycloak

import (
	"io"
	"net/http"
	"strconv"
	"time"
)

// retryTransport retries 429 and 5xx responses, honoring a numeric Retry-After
// header, with bounded exponential backoff. It is request-context aware.
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
		if !retryableStatus(resp.StatusCode) || attempt >= t.maxRetries {
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
