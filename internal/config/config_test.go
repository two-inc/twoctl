package config

import (
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// withTempHome routes config + keychain to test-scoped storage.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	keyring.MockInit()
	return dir
}

func TestInferURLBuiltins(t *testing.T) {
	cases := map[string]string{
		"prod":    "https://api.two.inc",
		"sandbox": "https://api.sandbox.two.inc",
		"staging": "https://api.staging.two.inc",
		"cyber":   "https://api.cyber.two.inc",
		"perf":    "https://api.perftest.two.inc",
		"release": "https://api.release.two.inc",
	}
	for in, want := range cases {
		if got := inferURL(in); got != want {
			t.Errorf("inferURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInferURLArbitrary(t *testing.T) {
	if got := inferURL("custom-env"); got != "https://api.custom-env.two.inc" {
		t.Errorf("inferURL custom = %q", got)
	}
}

func TestNormaliseURL(t *testing.T) {
	if got := normaliseURL("api.two.inc/"); got != "https://api.two.inc" {
		t.Errorf("normaliseURL bare = %q", got)
	}
	if got := normaliseURL("http://localhost:8080"); got != "http://localhost:8080" {
		t.Errorf("normaliseURL http = %q", got)
	}
}

func TestSetContextAutoFirstCurrent(t *testing.T) {
	withTempHome(t)
	if err := SetContext("sandbox", "", ""); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "sandbox" {
		t.Errorf("first context should become current, got %q", cfg.CurrentContext)
	}
	if cfg.Contexts["sandbox"].BaseURL != "https://api.sandbox.two.inc" {
		t.Errorf("URL not inferred: %+v", cfg.Contexts["sandbox"])
	}
}

func TestSetContextWithKey(t *testing.T) {
	withTempHome(t)
	if err := SetContext("cyber", "", "secret_test_xyz"); err != nil {
		t.Fatal(err)
	}
	if !HasStoredKey("cyber") {
		t.Error("key not stored")
	}
}

func TestSetContextSecondDoesNotChangeCurrent(t *testing.T) {
	withTempHome(t)
	_ = SetContext("first", "", "")
	_ = SetContext("second", "", "")
	cfg, _ := LoadFile()
	if cfg.CurrentContext != "first" {
		t.Errorf("current should remain 'first', got %q", cfg.CurrentContext)
	}
}

func TestUseContext(t *testing.T) {
	withTempHome(t)
	_ = SetContext("a", "", "")
	_ = SetContext("b", "", "")
	if err := UseContext("b"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadFile()
	if cfg.CurrentContext != "b" {
		t.Errorf("UseContext didn't switch, got %q", cfg.CurrentContext)
	}
	if err := UseContext("nope"); err == nil {
		t.Error("UseContext on missing should error")
	}
}

func TestDeleteContext(t *testing.T) {
	withTempHome(t)
	_ = SetContext("temp", "", "secret_test_k")
	if err := DeleteContext("temp"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadFile()
	if _, ok := cfg.Contexts["temp"]; ok {
		t.Error("context not deleted")
	}
	if HasStoredKey("temp") {
		t.Error("keychain entry not cleaned")
	}
	if err := DeleteContext("nope"); err == nil {
		t.Error("DeleteContext on missing should error")
	}
}

func TestListContextsSorted(t *testing.T) {
	withTempHome(t)
	_ = SetContext("b", "", "")
	_ = SetContext("a", "", "")
	_ = SetContext("c", "", "")
	contexts, current, err := ListContexts()
	if err != nil {
		t.Fatal(err)
	}
	if len(contexts) != 3 {
		t.Fatalf("got %d contexts", len(contexts))
	}
	if contexts[0].Name != "a" || contexts[1].Name != "b" || contexts[2].Name != "c" {
		t.Errorf("not sorted: %v", contexts)
	}
	if current != "b" {
		t.Errorf("current = %q, want b (first set)", current)
	}
}

func TestResolveFromFlag(t *testing.T) {
	withTempHome(t)
	os.Unsetenv("TWO_API_KEY")
	r, err := Resolve("secret_test_flag", "sandbox", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.APIKey != "secret_test_flag" {
		t.Errorf("APIKey = %q", r.APIKey)
	}
	if !strings.Contains(r.Source, "flag") {
		t.Errorf("Source = %q", r.Source)
	}
	if r.BaseURL != "https://api.sandbox.two.inc" {
		t.Errorf("BaseURL = %q", r.BaseURL)
	}
}

func TestResolveFromEnv(t *testing.T) {
	withTempHome(t)
	t.Setenv("TWO_API_KEY", "secret_prod_env")
	r, err := Resolve("", "prod", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.APIKey != "secret_prod_env" {
		t.Errorf("APIKey = %q", r.APIKey)
	}
	if !strings.Contains(r.Source, "TWO_API_KEY") {
		t.Errorf("Source = %q", r.Source)
	}
}

func TestResolveFromContext(t *testing.T) {
	withTempHome(t)
	os.Unsetenv("TWO_API_KEY")
	_ = SetContext("sandbox", "", "secret_test_keychain")
	r, err := Resolve("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.APIKey != "secret_test_keychain" {
		t.Errorf("APIKey = %q", r.APIKey)
	}
	if !strings.Contains(r.Source, "keychain") {
		t.Errorf("Source = %q", r.Source)
	}
	if r.ContextName != "sandbox" {
		t.Errorf("ContextName = %q", r.ContextName)
	}
}

func TestResolveNoKey(t *testing.T) {
	withTempHome(t)
	os.Unsetenv("TWO_API_KEY")
	_ = SetContext("sandbox", "", "")
	_, err := Resolve("", "", "")
	if err != ErrNoAPIKey {
		t.Errorf("err = %v, want ErrNoAPIKey", err)
	}
}

func TestResolveNoContext(t *testing.T) {
	withTempHome(t)
	os.Unsetenv("TWO_API_KEY")
	_, err := Resolve("secret_test_key", "", "")
	if err != ErrNoContext {
		t.Errorf("err = %v, want ErrNoContext", err)
	}
}

func TestResolveRawURLOverride(t *testing.T) {
	withTempHome(t)
	os.Unsetenv("TWO_API_KEY")
	r, err := Resolve("secret_test_flag", "", "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	if r.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q", r.BaseURL)
	}
}

func TestResolveArbitraryEnv(t *testing.T) {
	withTempHome(t)
	os.Unsetenv("TWO_API_KEY")
	r, err := Resolve("secret_test_flag", "cyber", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.BaseURL != "https://api.cyber.two.inc" {
		t.Errorf("cyber URL = %q", r.BaseURL)
	}
}
