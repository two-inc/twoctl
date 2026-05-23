package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func runPreprocess(t *testing.T, in string) string {
	t.Helper()
	dir := t.TempDir()
	inFile := filepath.Join(dir, "in.yaml")
	outFile := filepath.Join(dir, "out.yaml")
	if err := os.WriteFile(inFile, []byte(in), 0o600); err != nil {
		t.Fatal(err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(in), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	walk(&root)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		t.Fatalf("encode: %v", err)
	}
	enc.Close()
	_ = outFile
	return buf.String()
}

func TestDowngradeVersion(t *testing.T) {
	out := runPreprocess(t, "openapi: 3.1.0\ninfo:\n  title: x\n")
	if !strings.Contains(out, "3.0.3") {
		t.Errorf("openapi version not downgraded:\n%s", out)
	}
}

func TestAnyOfNullSingleBranch(t *testing.T) {
	in := `openapi: 3.1.0
field:
  anyOf:
    - type: string
    - type: 'null'
`
	out := runPreprocess(t, in)
	if !strings.Contains(out, "type: string") || !strings.Contains(out, "nullable: true") {
		t.Errorf("expected single-branch anyOf collapsed to type+nullable:\n%s", out)
	}
	if strings.Contains(out, "anyOf:") {
		t.Errorf("anyOf should have been removed:\n%s", out)
	}
}

func TestAnyOfNullMultiBranch(t *testing.T) {
	in := `openapi: 3.1.0
field:
  anyOf:
    - type: string
    - type: integer
    - type: 'null'
`
	out := runPreprocess(t, in)
	if !strings.Contains(out, "anyOf:") {
		t.Errorf("multi-branch anyOf should keep anyOf key:\n%s", out)
	}
	if !strings.Contains(out, "nullable: true") {
		t.Errorf("nullable not set:\n%s", out)
	}
	if strings.Contains(out, "type: 'null'") || strings.Contains(out, "type: \"null\"") {
		t.Errorf("null branch should have been removed:\n%s", out)
	}
}

func TestTypeArrayNull(t *testing.T) {
	in := `field:
  type:
    - string
    - 'null'
`
	out := runPreprocess(t, in)
	if !strings.Contains(out, "type: string") || !strings.Contains(out, "nullable: true") {
		t.Errorf("type array with null not collapsed:\n%s", out)
	}
}

func TestScalarTypeNull(t *testing.T) {
	in := `field:
  type: 'null'
`
	out := runPreprocess(t, in)
	if !strings.Contains(out, "nullable: true") {
		t.Errorf("bare type:null not converted:\n%s", out)
	}
	if strings.Contains(out, "type: 'null'") {
		t.Errorf("bare type:null not removed:\n%s", out)
	}
}

func TestConstPropertyMadeRequired(t *testing.T) {
	in := `schema:
  type: object
  properties:
    kind:
      const: NET_TERMS
      type: string
    other:
      type: string
`
	out := runPreprocess(t, in)
	if !strings.Contains(out, "required:") {
		t.Errorf("expected required: added:\n%s", out)
	}
	if !strings.Contains(out, "- kind") {
		t.Errorf("const property kind not added to required:\n%s", out)
	}
}

func TestConstPropertyAlreadyRequired(t *testing.T) {
	in := `schema:
  type: object
  required:
    - kind
  properties:
    kind:
      const: NET_TERMS
      type: string
`
	out := runPreprocess(t, in)
	// Should not duplicate.
	if strings.Count(out, "- kind") != 1 {
		t.Errorf("kind appears multiple times in required:\n%s", out)
	}
}

func TestIsNullSchema(t *testing.T) {
	null := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "type"},
			{Kind: yaml.ScalarNode, Value: "null"},
		},
	}
	if !isNullSchema(null) {
		t.Error("isNullSchema should return true for {type:null}")
	}
	notNull := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "type"},
			{Kind: yaml.ScalarNode, Value: "string"},
		},
	}
	if isNullSchema(notNull) {
		t.Error("isNullSchema should return false for {type:string}")
	}
}
