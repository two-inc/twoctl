package httpx

import (
	"net/http"
	"testing"
	"time"
)

func TestBackoffDelayClampsAttempt(t *testing.T) {
	// Pathological attempt values should not panic and should clamp at
	// maxBackoff (+ jitter).
	for _, attempt := range []int{0, 5, 10, 50, 100, 1000} {
		d := backoffDelay(attempt, nil)
		if d <= 0 {
			t.Errorf("attempt=%d returned non-positive delay %v", attempt, d)
		}
		if d > maxBackoff+maxBackoff/4 {
			t.Errorf("attempt=%d returned %v which exceeds the cap+jitter window", attempt, d)
		}
	}
}

func TestBackoffDelayHonoursRetryAfter(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Retry-After": []string{"7"}}}
	if d := backoffDelay(0, resp); d != 7*time.Second {
		t.Errorf("Retry-After: 7 → %v, want 7s", d)
	}
}

func TestBackoffDelayBadRetryAfterFallsBack(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Retry-After": []string{"not a number"}}}
	d := backoffDelay(0, resp)
	if d <= 0 {
		t.Errorf("garbage Retry-After should fall back to backoff, got %v", d)
	}
}
