package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewInjectsHeaders(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := New("secret_test_abc123")
	resp, err := client.Get(srv.URL + "/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got.Header.Get("X-Api-Key") != "secret_test_abc123" {
		t.Errorf("X-Api-Key not injected: %q", got.Header.Get("X-Api-Key"))
	}
	if !strings.HasPrefix(got.Header.Get("User-Agent"), "twoctl/") {
		t.Errorf("User-Agent missing/wrong: %q", got.Header.Get("User-Agent"))
	}
	if got.Header.Get("Accept") != "application/json" {
		t.Errorf("Accept = %q", got.Header.Get("Accept"))
	}
}

func TestRoundTripDoesNotOverrideExistingHeader(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := New("default-key")
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("X-Api-Key", "explicit-key")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.Header.Get("X-Api-Key") != "explicit-key" {
		t.Errorf("middleware overrode explicit X-Api-Key")
	}
}
