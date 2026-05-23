// preprocess rewrites OpenAPI 3.1 specs into a 3.0-compatible shape that
// oapi-codegen can consume.
//
// Two rewrites are applied recursively:
//
//  1. `type: [X, "null"]` -> `type: X` + `nullable: true`
//  2. `anyOf: [{...T...}, {type: "null"}]` -> the T schema + `nullable: true`
//
// The top-level `openapi:` version is also downgraded to 3.0.3.
//
// Usage: preprocess <input.yaml> <output.yaml>
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: preprocess <in.yaml> <out.yaml>")
		os.Exit(2)
	}
	in, err := os.ReadFile(os.Args[1])
	must(err)

	var root yaml.Node
	must(yaml.Unmarshal(in, &root))

	walk(&root)

	out, err := os.Create(os.Args[2])
	must(err)
	defer out.Close()
	enc := yaml.NewEncoder(out)
	enc.SetIndent(2)
	must(enc.Encode(&root))
	must(enc.Close())
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// walk visits every node in the document tree and applies rewrites.
func walk(n *yaml.Node) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			walk(c)
		}
		// Downgrade openapi version at the document root.
		if len(n.Content) > 0 && n.Content[0].Kind == yaml.MappingNode {
			downgradeVersion(n.Content[0])
		}
	case yaml.MappingNode:
		rewriteAnyOfNull(n)
		rewriteTypeArrayNull(n)
		rewriteScalarTypeNull(n)
		requireConstProperties(n)
		// Recurse after rewrite so we visit the resulting tree.
		for i := 1; i < len(n.Content); i += 2 {
			walk(n.Content[i])
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			walk(c)
		}
	}
}

func downgradeVersion(m *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Value == "openapi" && v.Kind == yaml.ScalarNode {
			v.Value = "3.0.3"
			return
		}
	}
}

// requireConstProperties marks any property carrying `const:` as required on
// its parent schema. A const value is by definition always present, and
// leaving it optional yields `*string` fields that break oapi-codegen's
// discriminator helpers (which assign the const as a plain string).
func requireConstProperties(m *yaml.Node) {
	var props, req *yaml.Node
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		switch k.Value {
		case "properties":
			props = v
		case "required":
			req = v
		}
	}
	if props == nil || props.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(props.Content); i += 2 {
		name, schema := props.Content[i], props.Content[i+1]
		if schema.Kind != yaml.MappingNode {
			continue
		}
		hasConst := false
		for j := 0; j+1 < len(schema.Content); j += 2 {
			if schema.Content[j].Value == "const" {
				hasConst = true
				break
			}
		}
		if !hasConst {
			continue
		}
		if req == nil {
			req = &yaml.Node{Kind: yaml.SequenceNode}
			m.Content = append(m.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "required"},
				req,
			)
		}
		exists := false
		for _, item := range req.Content {
			if item.Value == name.Value {
				exists = true
				break
			}
		}
		if !exists {
			req.Content = append(req.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: name.Value},
			)
		}
	}
}

// rewriteScalarTypeNull rewrites a bare `type: "null"` to `nullable: true`
// (dropping the type entirely - oapi-codegen has no representation for a
// pure-null schema).
func rewriteScalarTypeNull(m *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Value != "type" || v.Kind != yaml.ScalarNode || v.Value != "null" {
			continue
		}
		m.Content = append(m.Content[:i], m.Content[i+2:]...)
		setOrAdd(m, "nullable", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		return
	}
}

// rewriteTypeArrayNull collapses `type: [X, "null"]` to `type: X` + nullable.
func rewriteTypeArrayNull(m *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Value != "type" || v.Kind != yaml.SequenceNode {
			continue
		}
		var nonNull []*yaml.Node
		hasNull := false
		for _, c := range v.Content {
			if c.Kind == yaml.ScalarNode && c.Value == "null" {
				hasNull = true
				continue
			}
			nonNull = append(nonNull, c)
		}
		if !hasNull || len(nonNull) != 1 {
			continue
		}
		// Replace the sequence with the single remaining scalar.
		*v = *nonNull[0]
		setOrAdd(m, "nullable", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	}
}

// rewriteAnyOfNull collapses `anyOf: [T, {type: "null"}]` to T + nullable.
func rewriteAnyOfNull(m *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Value != "anyOf" && k.Value != "oneOf" {
			continue
		}
		if v.Kind != yaml.SequenceNode {
			continue
		}
		var nonNull []*yaml.Node
		hasNull := false
		for _, item := range v.Content {
			if isNullSchema(item) {
				hasNull = true
				continue
			}
			nonNull = append(nonNull, item)
		}
		if !hasNull {
			continue
		}
		switch len(nonNull) {
		case 0:
			// All-null anyOf is nonsense - drop the key, leave nullable.
			removeKey(m, k.Value)
			setOrAdd(m, "nullable", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		case 1:
			// Replace the anyOf with the surviving schema's contents.
			removeKey(m, k.Value)
			mergeInto(m, nonNull[0])
			setOrAdd(m, "nullable", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		default:
			// Multi-branch anyOf with a null - keep anyOf but drop the null branch.
			v.Content = nonNull
			setOrAdd(m, "nullable", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		}
		// Restart in case the merged-in schema introduced new keys we need to revisit.
		return
	}
}

func isNullSchema(n *yaml.Node) bool {
	if n.Kind != yaml.MappingNode {
		return false
	}
	// A schema is "null-only" if any `type: null` entry is present.
	// Sibling decorations (description, title, examples) are allowed -
	// the earlier exact-length check missed legitimate `{type: null,
	// description: "absent"}` shapes and left the null branch in the
	// output.
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Value == "type" && v.Kind == yaml.ScalarNode && v.Value == "null" {
			return true
		}
	}
	return false
}

func removeKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

func setOrAdd(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		val,
	)
}

// mergeInto copies key/value pairs from src mapping into dst mapping,
// skipping keys that already exist in dst.
func mergeInto(dst, src *yaml.Node) {
	if src.Kind != yaml.MappingNode {
		return
	}
	existing := map[string]bool{}
	for i := 0; i+1 < len(dst.Content); i += 2 {
		existing[dst.Content[i].Value] = true
	}
	for i := 0; i+1 < len(src.Content); i += 2 {
		if existing[src.Content[i].Value] {
			continue
		}
		dst.Content = append(dst.Content, src.Content[i], src.Content[i+1])
	}
}
