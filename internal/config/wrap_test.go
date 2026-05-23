package config

import (
	"errors"
	"strings"
	"testing"
)

func TestWrapKeyringErrPassesThrough(t *testing.T) {
	if got := wrapKeyringErr(nil); got != nil {
		t.Errorf("nil → %v", got)
	}
	orig := errors.New("kc unavailable")
	if got := wrapKeyringErr(orig); got != orig {
		t.Errorf("generic err should pass through unchanged: %v", got)
	}
}

func TestWrapKeyringErrFriendlyOnLinuxBackendMissing(t *testing.T) {
	for _, msg := range []string{
		"could not connect to secret-service",
		"org.freedesktop.secrets returned an error",
		"DBUS_SESSION_BUS_ADDRESS not set",
	} {
		got := wrapKeyringErr(errors.New(msg))
		if got == nil || !strings.Contains(got.Error(), "libsecret") {
			t.Errorf("backend-missing message not wrapped: %v", got)
		}
	}
}
