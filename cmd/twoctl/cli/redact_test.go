package cli

import (
	"strings"
	"testing"
)

func TestRedactSensitiveScalar(t *testing.T) {
	// Bare string that looks like an API key.
	in := "secret_test_aaaaaaaaaaaaaaaaaaaa"
	got, _ := redactSensitive(in).(string)
	if got == in {
		t.Errorf("API-key string not redacted: %q", got)
	}
}

func TestRedactSensitiveMap(t *testing.T) {
	in := map[string]any{
		"api_key":   "secret_test_aaaaaaaaaaaaaaaaaaaa",
		"plain":     "ok",
		"AuthToken": "deadbeef",
		"nested":    map[string]any{"password": "hunter2"},
		"array":     []any{"secret_prod_aaaaaaaaaaaaaaaaaaaa", "fine"},
	}
	out := redactSensitive(in).(map[string]any)
	if v, ok := out["api_key"].(string); !ok || strings.Contains(v, "aaaaaaaa") {
		t.Errorf("api_key not masked: %v", out["api_key"])
	}
	if out["plain"] != "ok" {
		t.Errorf("plain value mutated: %v", out["plain"])
	}
	if v, _ := out["AuthToken"].(string); v == "deadbeef" {
		t.Error("AuthToken not masked")
	}
	nested := out["nested"].(map[string]any)
	if v, _ := nested["password"].(string); v == "hunter2" {
		t.Error("nested password not masked")
	}
	arr := out["array"].([]any)
	if s, _ := arr[0].(string); strings.Contains(s, "aaaaaaaa") {
		t.Error("api-key inside array not masked")
	}
	if arr[1] != "fine" {
		t.Error("non-secret array element mutated")
	}
}

func TestRedactSensitivePassThrough(t *testing.T) {
	for _, v := range []any{nil, 42, true, 3.14} {
		if redactSensitive(v) != v {
			t.Errorf("scalar %v should pass through", v)
		}
	}
}
