package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := map[string]Format{
		"":      FormatTable,
		"TABLE": FormatTable,
		"json":  FormatJSON,
		"yaml":  FormatYAML,
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q) err: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Parse(%q) = %s, want %s", in, got, want)
		}
	}
	if _, err := Parse("bogus"); err == nil {
		t.Error("expected error for bogus format")
	}
}

func TestRenderJSONArray(t *testing.T) {
	body := []byte(`[{"a":1},{"a":2}]`)
	var buf bytes.Buffer
	if err := Render(&buf, FormatJSON, body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"a": 1`) || !strings.Contains(buf.String(), `"a": 2`) {
		t.Errorf("missing values:\n%s", buf.String())
	}
}

func TestRenderYAML(t *testing.T) {
	body := []byte(`{"k":"v","n":42}`)
	var buf bytes.Buffer
	if err := Render(&buf, FormatYAML, body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "k: v") {
		t.Errorf("missing k:v:\n%s", buf.String())
	}
}

func TestRenderTableFromArrayOfObjects(t *testing.T) {
	body := []byte(`[{"id":"a","status":"OPEN","amount":100},{"id":"b","status":"PAID","amount":200}]`)
	var buf bytes.Buffer
	if err := Render(&buf, FormatTable, body); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "AMOUNT") || !strings.Contains(out, "STATUS") || !strings.Contains(out, "ID") {
		t.Errorf("missing headers:\n%s", out)
	}
	if !strings.Contains(out, "OPEN") || !strings.Contains(out, "PAID") {
		t.Errorf("missing row data:\n%s", out)
	}
}

func TestRenderTableFallsBackToJSONOnNonArray(t *testing.T) {
	body := []byte(`{"k":"v"}`)
	var buf bytes.Buffer
	if err := Render(&buf, FormatTable, body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"k": "v"`) {
		t.Errorf("expected JSON fallback:\n%s", buf.String())
	}
}

func TestRenderTableSkipsHeterogeneousArrays(t *testing.T) {
	// Array of mixed scalars - should fall back to JSON not blow up.
	body := []byte(`["a", 1, true]`)
	var buf bytes.Buffer
	if err := Render(&buf, FormatTable, body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"a"`) {
		t.Errorf("expected JSON fallback for non-object array:\n%s", buf.String())
	}
}

func TestRenderRawNonJSONBody(t *testing.T) {
	body := []byte(`plain text response`)
	var buf bytes.Buffer
	if err := Render(&buf, FormatTable, body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "plain text response") {
		t.Errorf("non-JSON body not preserved:\n%s", buf.String())
	}
}

func TestRenderEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, FormatTable, nil); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output for empty body, got %q", buf.String())
	}
}

func TestRenderTableCellFormats(t *testing.T) {
	cases := map[any]string{
		nil:           "",
		"hello":       "hello",
		true:          "true",
		false:         "false",
		float64(42):   "42",
		float64(3.14): "3.14",
	}
	for in, want := range cases {
		if got := formatCell(in); got != want {
			t.Errorf("formatCell(%v) = %q, want %q", in, got, want)
		}
	}
	// Nested values get JSON encoded.
	got := formatCell(map[string]any{"k": "v"})
	if !strings.Contains(got, `"k":"v"`) {
		t.Errorf("nested object should JSON-encode, got %q", got)
	}
}

func TestRenderTableEmptyRows(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderTable(&buf, []string{"A", "B"}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "A") || !strings.Contains(buf.String(), "B") {
		t.Errorf("headers missing:\n%s", buf.String())
	}
}
