// Package output renders command results in the format selected by --output:
//
//   - table  (default, human-friendly aligned columns)
//   - json   (pretty-printed for piping into jq)
//   - yaml   (pretty-printed YAML)
//
// `Render` is the single entry point used by the operation runner. For a
// JSON array of objects it produces a table whose columns are the union of
// top-level keys across rows. For non-tabular shapes it falls back to JSON
// so the user always sees the full payload.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format names the three supported output modes.
type Format string

// Possible Format values.
const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// Parse normalises a user-supplied --output value. An empty string maps to
// FormatTable.
func Parse(raw string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(FormatTable):
		return FormatTable, nil
	case string(FormatJSON):
		return FormatJSON, nil
	case string(FormatYAML):
		return FormatYAML, nil
	}
	return "", fmt.Errorf("invalid --output %q: want one of table, json, yaml", raw)
}

// Render writes body to w in the requested format. body is the raw response
// bytes from the upstream API; the function decodes once and dispatches.
func Render(w io.Writer, format Format, body []byte) error {
	if len(body) == 0 {
		return nil
	}
	// json.Decoder.UseNumber so IDs > 2^53 don't silently lose precision
	// via float64. json.Number renders verbatim in all three formats.
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		// Not JSON - dump the raw bytes verbatim regardless of format.
		_, err := w.Write(body)
		if err != nil {
			return err
		}
		fmt.Fprintln(w)
		return nil
	}
	switch format {
	case FormatYAML:
		return renderYAML(w, v)
	case FormatJSON:
		return renderJSON(w, v)
	}
	// Table: only meaningful for an array of objects. Anything else falls
	// back to JSON so the user always sees the full payload.
	if rows, headers, ok := tabularise(v); ok {
		return RenderTable(w, headers, rows)
	}
	return renderJSON(w, v)
}

func renderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func renderYAML(w io.Writer, v any) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer enc.Close()
	return enc.Encode(v)
}

// tabularise tries to turn v into (rows, headers) for table rendering.
// Returns ok=false if v isn't an array of homogeneous-ish objects.
func tabularise(v any) ([][]string, []string, bool) {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return nil, nil, false
	}
	keySet := map[string]struct{}{}
	for _, row := range arr {
		obj, ok := row.(map[string]any)
		if !ok {
			return nil, nil, false
		}
		for k := range obj {
			keySet[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([][]string, 0, len(arr))
	for _, row := range arr {
		obj := row.(map[string]any)
		cells := make([]string, len(keys))
		for i, k := range keys {
			cells[i] = formatCell(obj[k])
		}
		rows = append(rows, cells)
	}
	return rows, keys, true
}

// formatCell renders a single value for a table cell. Scalars use their
// natural string form; nested objects/arrays are JSON-encoded so the table
// stays one row per record.
func formatCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case json.Number:
		return x.String()
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// RenderTable writes an aligned table. Each row has len == len(headers); short
// rows are padded with empty cells, long rows are truncated to header width.
func RenderTable(w io.Writer, headers []string, rows [][]string) error {
	cols := len(headers)
	widths := make([]int, cols)
	upper := make([]string, cols)
	for i, h := range headers {
		upper[i] = strings.ToUpper(h)
		widths[i] = len(upper[i])
	}
	norm := make([][]string, 0, len(rows))
	for _, r := range rows {
		row := make([]string, cols)
		for i := 0; i < cols; i++ {
			if i < len(r) {
				row[i] = r[i]
			}
			if l := displayLen(row[i]); l > widths[i] {
				widths[i] = l
			}
		}
		norm = append(norm, row)
	}
	writeRow := func(cells []string) error {
		parts := make([]string, cols)
		for i, c := range cells {
			if i == cols-1 {
				parts[i] = c
			} else {
				parts[i] = c + strings.Repeat(" ", widths[i]-displayLen(c))
			}
		}
		_, err := fmt.Fprintln(w, strings.Join(parts, "  "))
		return err
	}
	if err := writeRow(upper); err != nil {
		return err
	}
	for _, r := range norm {
		if err := writeRow(r); err != nil {
			return err
		}
	}
	return nil
}

// displayLen treats every rune as width 1. Good enough for the ASCII-ish
// content we see in Two API responses.
func displayLen(s string) int { return len([]rune(s)) }
