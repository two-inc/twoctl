package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestHasPaginationParam(t *testing.T) {
	op := &openapi3.Operation{Parameters: openapi3.Parameters{
		&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "page_cursor", In: "query"}},
	}}
	if !hasPaginationParam(op) {
		t.Error("expected pagination detected")
	}
	op2 := &openapi3.Operation{Parameters: openapi3.Parameters{
		&openapi3.ParameterRef{Value: &openapi3.Parameter{Name: "limit", In: "query"}},
	}}
	if hasPaginationParam(op2) {
		t.Error("limit alone should not count")
	}
}

func TestSplitPage(t *testing.T) {
	body := []byte(`{"items":[{"id":1},{"id":2}],"next_page_cursor":"c2"}`)
	items, next := splitPage(body)
	if len(items) != 2 || next != "c2" {
		t.Errorf("got items=%v next=%q", items, next)
	}
	// Missing fields tolerated.
	items, next = splitPage([]byte(`{}`))
	if items != nil || next != "" {
		t.Errorf("empty page should yield nil/empty, got %v/%q", items, next)
	}
	// Bad JSON returns nil/empty.
	items, next = splitPage([]byte(`garbage`))
	if items != nil || next != "" {
		t.Errorf("garbage should yield nil/empty, got %v/%q", items, next)
	}
}

func TestRunPaginatedFollowsCursor(t *testing.T) {
	resetGlobals(t)
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case 1:
			fmt.Fprint(w, `{"items":[{"id":"a"},{"id":"b"}],"next_page_cursor":"c2"}`)
		case 2:
			if got := r.URL.Query().Get("page_cursor"); got != "c2" {
				t.Errorf("expected page_cursor=c2 on page 2, got %q", got)
			}
			fmt.Fprint(w, `{"items":[{"id":"c"}],"next_page_cursor":""}`)
		default:
			t.Errorf("unexpected extra page request (#%d)", page)
		}
	}))
	defer srv.Close()

	stdout, _, err := runRoot(t, "--url", srv.URL, "--api-key", "secret_test_x", "-o", "json",
		"billing-statement", "get", "--all")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	items, _ := got["items"].([]any)
	if len(items) != 3 {
		t.Errorf("expected 3 items concatenated across pages, got %d:\n%s", len(items), stdout)
	}
	if page != 2 {
		t.Errorf("expected 2 server calls, got %d", page)
	}
}

func TestRunPaginatedHTTPError(t *testing.T) {
	resetGlobals(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()

	_, _, err := runRoot(t, "--url", srv.URL, "--api-key", "secret_test_x",
		"billing-statement", "get", "--all")
	ae, ok := err.(*apiError)
	if !ok {
		t.Fatalf("expected *apiError, got %T: %v", err, err)
	}
	if ae.exit != ExitAuth {
		t.Errorf("exit = %d, want %d", ae.exit, ExitAuth)
	}
}

func TestRequestIDCaptured(t *testing.T) {
	resetGlobals(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req_abc123")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"missing"}`))
	}))
	defer srv.Close()

	_, _, err := runRoot(t, "--url", srv.URL, "--api-key", "secret_test_x",
		"order", "get", "--order-id", "x")
	ae, ok := err.(*apiError)
	if !ok {
		t.Fatalf("expected *apiError, got %T", err)
	}
	if ae.requestID != "req_abc123" {
		t.Errorf("request_id not captured: %q", ae.requestID)
	}
}

func TestCaptureRequestIDFallback(t *testing.T) {
	h := http.Header{}
	h.Set("X-Trace-Id", "trace-xyz")
	if got := captureRequestID(h); got != "trace-xyz" {
		t.Errorf("got %q", got)
	}
	if got := captureRequestID(http.Header{}); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFriendlyHTTPMessage(t *testing.T) {
	cases := []struct {
		status int
		want   string // substring
	}{
		{http.StatusUnauthorized, "twoctl auth login"},
		{http.StatusForbidden, "does not have access"},
		{http.StatusNotFound, "not found"},
		{http.StatusTooManyRequests, "rate limited"},
		{http.StatusBadGateway, "server error"},
		{http.StatusBadRequest, "HTTP 400"},
	}
	for _, c := range cases {
		got := friendlyHTTPMessage(c.status)
		if !strings.Contains(got, c.want) {
			t.Errorf("friendlyHTTPMessage(%d) = %q, want substring %q", c.status, got, c.want)
		}
	}
}
