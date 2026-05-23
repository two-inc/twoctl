package cli

import (
	"strings"
	"testing"

	"github.com/two-inc/twoctl-cli/internal/config"
)

func TestWhoamiOutputs(t *testing.T) {
	resetGlobals(t)
	_ = config.SetContext("sandbox", "", "secret_test_aaaaaaaaaaaaaaaa1234")
	resetCobraFlags(Root())

	stdout, _, err := runRoot(t, "auth", "whoami")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "sandbox") || !strings.Contains(stdout, "api.sandbox.two.inc") {
		t.Errorf("whoami missing context/url:\n%s", stdout)
	}
	if !strings.Contains(stdout, "****1234") {
		t.Errorf("whoami missing redacted key suffix:\n%s", stdout)
	}
}

func TestResourceFromPath(t *testing.T) {
	cases := map[string]string{
		"/v1/order/{order_id}":          "order",
		"/v2/billing-account/{id}/keys": "billing-account-keys",
		"/v1/{id}":                      "",
	}
	for in, want := range cases {
		if got := resourceFromPath(in); got != want {
			t.Errorf("resourceFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDescribeRequestBodyFromCreate(t *testing.T) {
	resetGlobals(t)
	stdout, _, err := runRoot(t, "order", "create", "--describe")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"request_body"`) {
		t.Errorf("create describe should include request_body:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"responses"`) {
		t.Errorf("create describe should include responses:\n%s", stdout)
	}
}
