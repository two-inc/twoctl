package cli

import (
	"strings"
	"testing"

	"github.com/two-inc/twoctl-cli/internal/config"
)

func TestConfigSetAndUseContext(t *testing.T) {
	resetGlobals(t)
	if _, _, err := runRoot(t, "config", "set-context", "sandbox", "--key", "secret_test_x"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRoot(t, "config", "set-context", "prod"); err != nil {
		t.Fatal(err)
	}
	resetCobraFlags(Root())
	stdout, _, err := runRoot(t, "config", "current-context")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout) != "sandbox" {
		t.Errorf("current = %q, want sandbox", strings.TrimSpace(stdout))
	}
	resetCobraFlags(Root())
	if _, _, err := runRoot(t, "config", "use-context", "prod"); err != nil {
		t.Fatal(err)
	}
	resetCobraFlags(Root())
	stdout, _, _ = runRoot(t, "config", "current-context")
	if strings.TrimSpace(stdout) != "prod" {
		t.Errorf("after use-context, current = %q", strings.TrimSpace(stdout))
	}
}

func TestConfigGetContexts(t *testing.T) {
	resetGlobals(t)
	_ = config.SetContext("sandbox", "", "secret_test_x")
	_ = config.SetContext("prod", "", "")
	stdout, _, err := runRoot(t, "config", "get-contexts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "sandbox") || !strings.Contains(stdout, "prod") {
		t.Errorf("missing contexts:\n%s", stdout)
	}
}

func TestConfigDeleteContext(t *testing.T) {
	resetGlobals(t)
	_ = config.SetContext("temp", "", "")
	if _, _, err := runRoot(t, "config", "delete-context", "temp"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.LoadFile()
	if _, ok := cfg.Contexts["temp"]; ok {
		t.Error("context not removed")
	}
}

func TestConfigGetContextsEmpty(t *testing.T) {
	resetGlobals(t)
	stdout, _, err := runRoot(t, "config", "get-contexts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "no contexts") {
		t.Errorf("empty list missing hint:\n%s", stdout)
	}
}

func TestVersionCommand(t *testing.T) {
	resetGlobals(t)
	stdout, _, err := runRoot(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "twoctl version") || !strings.Contains(stdout, "go ") {
		t.Errorf("version output missing fields:\n%s", stdout)
	}
}

func TestExplainRoutesToDescribe(t *testing.T) {
	resetGlobals(t)
	stdout, _, err := runRoot(t, "explain", "order", "get")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"operation_id"`) {
		t.Errorf("explain didn't emit describe doc:\n%s", stdout)
	}
}
