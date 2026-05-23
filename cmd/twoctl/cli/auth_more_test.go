package cli

import "testing"

func TestLooksLikeAPIKeyStrict(t *testing.T) {
	cases := map[string]bool{
		"secret_test_aaaaaaaaaaaaaaaa":        true,
		"secret_prod_abcdefghij1234567890":    true,
		"secret_live_abcdefghij1234567890":    true,
		"secret_sandbox_abcdefghij1234567890": true,
		// too short
		"secret_test_x":   false,
		"secret_prod_abc": false,
		// embedded control char
		"secret_test_aaaaaaaaaa\naaaaaa": false,
		// wrong prefix
		"pk_test_aaaaaaaaaaaaaaaa": false,
		"":                         false,
	}
	for in, want := range cases {
		if got := looksLikeAPIKey(in); got != want {
			t.Errorf("looksLikeAPIKey(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRedactKeyConservativeOnGarbage(t *testing.T) {
	// Non-prefix inputs should redact to bare **** with no tail leakage.
	cases := []string{
		"random-garbage-input-not-an-api-key-at-all",
		"short",
		"",
		"_foo_bar_baz_",
	}
	for _, in := range cases {
		if got := redactKey(in); got != "****" {
			t.Errorf("redactKey(%q) = %q, want ****", in, got)
		}
	}
}
