package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/textproto"
	"os"
	"regexp"
	"strings"
)

// Exit codes - documented in README so scripts and agents can branch on them.
const (
	ExitOK          = 0
	ExitGeneric     = 1
	ExitUsage       = 2 // bad flags / missing required input
	ExitAuth        = 3 // 401/403
	ExitNotFound    = 4 // 404
	ExitRateLimited = 5 // 429
	ExitServer      = 6 // 5xx
	ExitNetwork     = 7 // transport-level failure
)

// apiError carries enough context to render a structured stderr envelope and
// exit with a meaningful code.
type apiError struct {
	code      string
	status    int
	message   string
	body      []byte
	requestID string
	exit      int
}

func (e *apiError) Error() string { return e.message }

func exitForStatus(s int) int {
	switch {
	case s == http.StatusUnauthorized, s == http.StatusForbidden:
		return ExitAuth
	case s == http.StatusNotFound:
		return ExitNotFound
	case s == http.StatusTooManyRequests:
		return ExitRateLimited
	case s >= 500:
		return ExitServer
	}
	return ExitGeneric
}

// HandleError renders any error returned from a command and exits with the
// right code. apiErrors are serialised to stderr as JSON so agents can parse
// them; other errors print as plain text.
func HandleError(err error) {
	if err == nil {
		os.Exit(ExitOK)
	}
	if ae, ok := err.(*apiError); ok {
		out := map[string]any{
			"error":   ae.code,
			"message": ae.message,
		}
		if ae.status != 0 {
			out["status"] = ae.status
		}
		if ae.requestID != "" {
			out["request_id"] = ae.requestID
		}
		if len(ae.body) > 0 {
			var parsed any
			if json.Unmarshal(ae.body, &parsed) == nil {
				out["response"] = redactSensitive(parsed)
			} else {
				out["response"] = string(ae.body)
			}
		}
		enc := json.NewEncoder(os.Stderr)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		os.Exit(ae.exit)
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(ExitGeneric)
}

// sensitiveHeaderRe matches headers we always redact in --dry-run output.
// Substring-matched against the canonical header name (case-folded by
// textproto.CanonicalMIMEHeaderKey).
var sensitiveHeaderRe = regexp.MustCompile(`(?i)(api[_-]?key|authoriz|cookie|secret|token|signature|password|csrf)`)

// redactHeaders returns a copy of h with sensitive values masked. Used by
// --dry-run output. The allowlist is sensitive-name regex rather than a
// fixed list, so a spec adding `X-Customer-Token` or `X-Signature` is
// automatically covered.
func redactHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for k, vs := range h {
		if len(vs) == 0 {
			continue
		}
		v := vs[0]
		canonical := textproto.CanonicalMIMEHeaderKey(k)
		if sensitiveHeaderRe.MatchString(canonical) {
			v = maskValue(v)
		}
		out[canonical] = v
	}
	return out
}

func maskValue(v string) string {
	if len(v) > 8 {
		return v[:4] + "****" + v[len(v)-4:]
	}
	return "****"
}

// sensitiveKeyRe matches JSON keys whose values we redact when echoing a
// server error response to stderr. Some upstream APIs anti-pattern-echo the
// key they just rejected back to the caller; we don't want it landing in
// shell history or CI logs.
var sensitiveKeyRe = regexp.MustCompile(`(?i)(api[_-]?key|authoriz|secret|token|password|signature)`)

// secretValueRe also redacts string values that look like Two API keys, even
// when their containing key isn't on the sensitive-name list.
var secretValueRe = regexp.MustCompile(`secret_(test|prod|live|sandbox)_[A-Za-z0-9_\-]+`)

// redactSensitive walks a decoded JSON value and masks any string under a
// sensitive-looking key, or any string that itself looks like an API key.
func redactSensitive(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if s, ok := val.(string); ok && sensitiveKeyRe.MatchString(k) {
				out[k] = maskValue(s)
				continue
			}
			out[k] = redactSensitive(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = redactSensitive(item)
		}
		return out
	case string:
		return secretValueRe.ReplaceAllStringFunc(x, maskValue)
	}
	return v
}

// guard import so refactors don't accidentally drop net/textproto.
var _ = strings.ToLower
