package config

import (
	"strings"
	"testing"
)

func TestSafeURLAcceptsHTTPS(t *testing.T) {
	got, err := safeURL("https://api.two.inc")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://api.two.inc" {
		t.Errorf("got %q", got)
	}
}

func TestSafeURLAddsHTTPSScheme(t *testing.T) {
	got, err := safeURL("api.two.inc")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://api.two.inc" {
		t.Errorf("got %q", got)
	}
}

func TestSafeURLAllowsLocalhostHTTP(t *testing.T) {
	if _, err := safeURL("http://localhost:8080"); err != nil {
		t.Errorf("localhost http should be allowed: %v", err)
	}
	if _, err := safeURL("http://127.0.0.1:9999"); err != nil {
		t.Errorf("127.0.0.1 http should be allowed: %v", err)
	}
}

func TestSafeURLRejectsRemoteHTTP(t *testing.T) {
	_, err := safeURL("http://attacker.example")
	if err == nil || !strings.Contains(err.Error(), "plaintext") {
		t.Errorf("plaintext url should be rejected, got %v", err)
	}
}

func TestSafeURLRejectsQueryFragmentUserinfo(t *testing.T) {
	for _, in := range []string{
		"https://api.two.inc?foo=bar",
		"https://api.two.inc#frag",
		"https://user:pass@api.two.inc",
	} {
		if _, err := safeURL(in); err == nil {
			t.Errorf("%q should have been rejected", in)
		}
	}
}

func TestSafeURLRejectsNoHost(t *testing.T) {
	if _, err := safeURL("https://?q=1"); err == nil {
		t.Error("missing host should be rejected")
	}
}
