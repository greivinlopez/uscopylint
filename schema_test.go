package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Model only the keywords in the published contract. Reject new keywords
// until the test knows how to exercise them.
type schemaNode struct {
	Dialect     string                `json:"$schema"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Type        string                `json:"type"`
	Items       *schemaNode           `json:"items"`
	Properties  map[string]schemaNode `json:"properties"`
	Required    []string              `json:"required"`
	Minimum     int64                 `json:"minimum"`
}

func (s schemaNode) check(t *testing.T, v any) {
	t.Helper()
	switch s.Type {
	case "array":
		values, ok := v.([]any)
		if !ok || s.Items == nil {
			t.Fatalf("expected array with an item schema, got %v", v)
		}
		for _, item := range values {
			s.Items.check(t, item)
		}
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("expected object, got %v", v)
		}
		for _, key := range s.Required {
			if _, exists := obj[key]; !exists {
				t.Fatalf("missing required property %q in %v", key, obj)
			}
		}
		for key, val := range obj {
			prop, documented := s.Properties[key]
			if !documented {
				t.Fatalf("CLI emits undocumented property %q", key)
			}
			prop.check(t, val)
		}
	case "string":
		if _, ok := v.(string); !ok {
			t.Fatalf("expected string, got %v", v)
		}
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			t.Fatalf("expected integer, got %v", v)
		}
		i, err := n.Int64()
		if err != nil || i < s.Minimum {
			t.Fatalf("expected integer >= %d, got %v", s.Minimum, v)
		}
	default:
		t.Fatalf("unexpected schema type %q", s.Type)
	}
}

func TestJSONOutputSchema(t *testing.T) {
	data, err := os.ReadFile("findings.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema schemaNode
	definition := json.NewDecoder(bytes.NewReader(data))
	definition.DisallowUnknownFields()
	if err := definition.Decode(&schema); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		src    string
		status int
	}{
		{"<p>Color</p>", 0},
		{"<p>💳 Colour</p>", 1},
		{"<p>Shopping tro<b>lley</b></p>", 1},
		{"<p>cash{' '}<b>machine</b></p>", 1},
	} {
		var out, diag bytes.Buffer
		if code := run([]string{"--output", "JSON", "--stdin-name", "Cart.tsx", "-"}, strings.NewReader(tc.src), &out, &diag); code != tc.status {
			t.Fatalf("%q: exit %d, %s", tc.src, code, diag.String())
		}
		var value any
		d := json.NewDecoder(&out)
		d.UseNumber()
		if err := d.Decode(&value); err != nil {
			t.Fatal(err)
		}
		schema.check(t, value)
	}
}
