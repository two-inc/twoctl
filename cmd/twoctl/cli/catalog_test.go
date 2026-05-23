package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestCollectCatalogAll(t *testing.T) {
	ops, err := collectCatalog("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) < 50 {
		t.Errorf("expected 50+ operations, got %d", len(ops))
	}
	actions := map[string]int{}
	for _, o := range ops {
		actions[o.Action]++
	}
	for _, a := range []string{"get", "create"} {
		if actions[a] == 0 {
			t.Errorf("action %s has no operations", a)
		}
	}
}

func TestCollectCatalogFilteredByAction(t *testing.T) {
	ops, err := collectCatalog("", "get")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) == 0 {
		t.Fatal("get action should have operations")
	}
	for _, o := range ops {
		if o.Action != "get" {
			t.Errorf("filter leaked action: %s", o.Action)
		}
	}
}

func TestCollectCatalogFilteredByResource(t *testing.T) {
	ops, err := collectCatalog("order", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) == 0 {
		t.Fatal("order resource should have operations")
	}
	for _, o := range ops {
		if o.Resource != "order" {
			t.Errorf("filter leaked resource: %s", o.Resource)
		}
	}
}

func TestRunAPIResourcesEmitsValidJSON(t *testing.T) {
	cmd := Root()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"api-resources", "--resource", "order"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var parsed []catalogOp
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, buf.String())
	}
	if len(parsed) == 0 {
		t.Fatal("expected order operations in output")
	}
	for _, o := range parsed {
		if !strings.HasPrefix(o.Command, "twoctl order ") {
			t.Errorf("bad command prefix: %q", o.Command)
		}
	}
}

func TestFlagTypeOf(t *testing.T) {
	cases := []struct {
		schema *openapi3.Schema
		want   string
	}{
		{nil, "string"},
		{&openapi3.Schema{}, "string"},
		{&openapi3.Schema{Type: &openapi3.Types{"integer"}}, "integer"},
		{&openapi3.Schema{Type: &openapi3.Types{"boolean"}}, "boolean"},
	}
	for _, c := range cases {
		p := &openapi3.Parameter{}
		if c.schema != nil {
			p.Schema = &openapi3.SchemaRef{Value: c.schema}
		}
		if got := flagTypeOf(p); got != c.want {
			t.Errorf("flagTypeOf(%+v) = %q, want %q", c.schema, got, c.want)
		}
	}
}

func TestLoadEmbeddedSpec(t *testing.T) {
	for _, m := range apiMeta {
		doc, err := loadEmbeddedSpec(m.file)
		if err != nil {
			t.Errorf("loadEmbeddedSpec(%s): %v", m.file, err)
			continue
		}
		if doc == nil || doc.Paths == nil {
			t.Errorf("%s: empty doc", m.file)
		}
	}
}
