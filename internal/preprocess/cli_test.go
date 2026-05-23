package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMainEndToEnd exercises the binary's main() function by setting os.Args
// to a real input/output pair and invoking main() in the same process. We
// use t.Cleanup to put argv back.
func TestMainEndToEnd(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.yaml")
	out := filepath.Join(dir, "out.yaml")
	body := `openapi: 3.1.0
info:
  title: x
  version: '1'
paths:
  /a:
    get:
      operationId: a
      parameters:
        - name: q
          in: query
          schema:
            anyOf:
              - type: string
              - type: 'null'
      responses:
        '200':
          description: ok
`
	if err := os.WriteFile(in, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = []string{"preprocess", in, out}
	main()
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "3.0.3") {
		t.Errorf("openapi not downgraded:\n%s", got)
	}
	if !strings.Contains(string(got), "nullable: true") {
		t.Errorf("anyOf null not collapsed:\n%s", got)
	}
}
