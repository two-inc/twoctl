package cli

// Resource-first command tree: `twoctl <resource> <action> [flags]`.
//
// Every operation across every embedded spec is classified into a (resource,
// action) pair derived from the operationId, then registered as
// `twoctl <resource> <action>`. This matches gh/stripe/aws CLI shape rather
// than kubectl's verb-first model, which lets non-CRUD actions (fulfill,
// refund, confirm, cancel, search, ...) surface as first-class verbs on the
// resource they operate on.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
)

// knownActions is the canonical action vocabulary, ordered by preference. The
// first matching prefix (or suffix) wins. Anything outside this list still
// falls through via the HTTP-method fall-back, but having an explicit list
// lets us recognise non-CRUD actions and keep their natural names.
var knownActions = []string{
	"get", "retrieve", "fetch", "list", "show", "find", "search", "lookup", "download",
	"create", "make", "register", "issue", "add", "post", "send", "generate",
	"update", "edit", "patch", "replace", "set", "configure",
	"delete", "remove", "revoke", "archive",
	"cancel", "refund", "fulfill", "fulfil", "confirm", "renew",
	"notify", "mark", "complete", "invoice",
}

// methodAction maps HTTP methods to a sensible action name when the
// operationId provides no usable prefix.
var methodAction = map[string]string{
	"GET":    "get",
	"POST":   "create",
	"DELETE": "delete",
	"PUT":    "update",
	"PATCH":  "patch",
}

func init() {
	parents := map[string]*cobra.Command{}
	taken := map[string]map[string]bool{}
	// resourceOwner tracks which spec first claimed each top-level
	// resource name. A second spec landing on the same resource gets a
	// qualified name (e.g. autofill-delegation vs registry-delegation) so
	// commands stay deterministic.
	resourceOwner := map[string]string{}

	for _, m := range apiMeta {
		apiStem := strings.TrimSuffix(strings.TrimSuffix(m.file, ".processed.yaml"), "-api")
		doc, err := loadEmbeddedSpec(m.file)
		if err != nil {
			continue
		}
		for _, e := range walkOperations(doc) {
			resource, action := classifyOperation(e.op, e.method, e.path)
			if resource == "" || action == "" {
				continue
			}
			if owner, claimed := resourceOwner[resource]; claimed && owner != apiStem {
				// Cross-spec collision: qualify the new resource with
				// its API stem so neither spec silently shadows the
				// other.
				resource = apiStem + "-" + resource
			} else {
				resourceOwner[resource] = apiStem
			}
			parent := parents[resource]
			if parent == nil {
				parent = &cobra.Command{
					Use:   resource,
					Short: fmt.Sprintf("Operations on %s", strings.ReplaceAll(resource, "-", " ")),
				}
				parents[resource] = parent
			}
			if taken[resource] == nil {
				taken[resource] = map[string]bool{}
			}
			name := action
			if taken[resource][name] {
				// Stable disambiguator: append the kebabed operationId
				// stem (or method+path if there is no operationId).
				// Avoids "rename on next spec edit" churn.
				suffix := toKebab(stripPythonLocals(strings.ToLower(e.op.OperationID)))
				if suffix == "" {
					suffix = toKebab(strings.ToLower(e.method) + "-" + e.path)
				}
				name = action + "-" + suffix
			}
			// Even with a stable suffix, two ops can collide; fall back
			// to a counter as a last resort.
			for n := 2; taken[resource][name]; n++ {
				name = fmt.Sprintf("%s-%d", name, n)
			}
			taken[resource][name] = true
			parent.AddCommand(buildOperationCommand(name, e.path, e.method, e.op))
		}
	}
	for _, p := range parents {
		if len(p.Commands()) > 0 {
			register(p)
		}
	}
}

// classifyOperation returns (kebab-resource, kebab-action) for an operation,
// or empty strings if it can't be classified.
//
// The resource is derived from the URL path so that
// /v1/order/{id}/cancel -> resource=order (action stripped if it appears as
// the trailing path segment); the operationId only contributes the action.
func classifyOperation(op *openapi3.Operation, method, path string) (string, string) {
	id := strings.ToLower(op.OperationID)
	id = trimKnownSuffixes(id)

	action := actionFromOperationID(id, method)
	if action == "" {
		return "", ""
	}
	resource := resourceFromPath(path)
	if resource == "" {
		resource = deriveResource(id, path)
	}
	// If the path's last segment is the action itself (e.g. /order/{id}/cancel),
	// strip it - the action lives in the leaf command, not the resource.
	if suffix := "-" + action; strings.HasSuffix(resource, suffix) {
		resource = strings.TrimSuffix(resource, suffix)
	}
	if resource == "" {
		return "", ""
	}
	return resource, action
}

// actionFromOperationID picks an action verb from the operationId (after
// trimKnownSuffixes), falling back to the HTTP method when no verb is found.
func actionFromOperationID(id, method string) string {
	for _, action := range knownActions {
		if strings.HasPrefix(id, action+"_") || id == action {
			return action
		}
	}
	for _, action := range knownActions {
		if strings.HasSuffix(id, "_"+action) {
			return action
		}
	}
	if action, ok := methodAction[method]; ok {
		return action
	}
	return ""
}

// stripPythonLocals removes the `make_X.<locals>.Y` pattern that some
// codegens emit as an operationId, keeping only the inner function name.
func stripPythonLocals(id string) string {
	if i := strings.Index(id, ".<locals>."); i >= 0 {
		return id[i+len(".<locals>."):]
	}
	return id
}

// trimKnownSuffixes drops trailing noise (HTTP method, _handler, version)
// from an operationId so the remaining stem is just the meaningful tokens.
func trimKnownSuffixes(id string) string {
	id = stripPythonLocals(id)
	for changed := true; changed; {
		changed = false
		for _, s := range []string{
			"_handler_get", "_handler_post", "_handler_put",
			"_handler_delete", "_handler_patch",
			"_handler", "_v1", "_v2",
			".get", ".post", ".put", ".delete", ".patch",
			"_get", "_post", "_put", "_delete", "_patch",
		} {
			if next := strings.TrimSuffix(id, s); next != id {
				id = next
				changed = true
			}
		}
		id = strings.TrimRight(id, "_")
	}
	return id
}

// deriveResource picks a stable kebab-case resource name from the stripped
// operationId. If the result is uninformative (empty or filler word) it
// falls back to a name derived from the URL path.
func deriveResource(stem, path string) string {
	stem = strings.Trim(stem, "_")
	stem = strings.ReplaceAll(stem, "__", "_")
	if stem == "" || stem == "by" || stem == "for" {
		return resourceFromPath(path)
	}
	return strings.ReplaceAll(stem, "_", "-")
}

// resourceFromPath builds a kebab-case resource name from the URL path by
// concatenating non-templated segments (skipping placeholders and version
// prefixes).
var versionSegRe = regexp.MustCompile(`^v\d+$`)

func resourceFromPath(path string) string {
	var parts []string
	for _, seg := range strings.Split(path, "/") {
		if seg == "" || strings.HasPrefix(seg, "{") {
			continue
		}
		if versionSegRe.MatchString(seg) {
			continue
		}
		parts = append(parts, strings.ReplaceAll(seg, "_", "-"))
	}
	return strings.Join(parts, "-")
}
