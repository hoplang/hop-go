package parser

import (
	"testing"

	"golang.org/x/net/html"
)

func TestParseAttributePositions(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     map[string]struct {
			nameStart  Position
			nameEnd    Position
			valueStart Position
			valueEnd   Position
		}
	}{
		{
			name:     "simple attribute",
			template: `<div class="foo"></div>`,
			want: map[string]struct {
				nameStart  Position
				nameEnd    Position
				valueStart Position
				valueEnd   Position
			}{
				"class": {
					nameStart:  Position{Line: 1, Column: 6},
					nameEnd:    Position{Line: 1, Column: 11},
					valueStart: Position{Line: 1, Column: 13},
					valueEnd:   Position{Line: 1, Column: 16},
				},
			},
		},
		{
			name:     "multiple attributes",
			template: `<div class="foo" id="bar"></div>`,
			want: map[string]struct {
				nameStart  Position
				nameEnd    Position
				valueStart Position
				valueEnd   Position
			}{
				"class": {
					nameStart:  Position{Line: 1, Column: 6},
					nameEnd:    Position{Line: 1, Column: 11},
					valueStart: Position{Line: 1, Column: 13},
					valueEnd:   Position{Line: 1, Column: 16},
				},
				"id": {
					nameStart:  Position{Line: 1, Column: 18},
					nameEnd:    Position{Line: 1, Column: 20},
					valueStart: Position{Line: 1, Column: 22},
					valueEnd:   Position{Line: 1, Column: 25},
				},
			},
		},
		{
			name: "multiline attributes",
			template: `<div
    class="foo"
    id="bar"
></div>`,
			want: map[string]struct {
				nameStart  Position
				nameEnd    Position
				valueStart Position
				valueEnd   Position
			}{
				"class": {
					nameStart:  Position{Line: 2, Column: 5},
					nameEnd:    Position{Line: 2, Column: 10},
					valueStart: Position{Line: 2, Column: 12},
					valueEnd:   Position{Line: 2, Column: 15},
				},
				"id": {
					nameStart:  Position{Line: 3, Column: 5},
					nameEnd:    Position{Line: 3, Column: 7},
					valueStart: Position{Line: 3, Column: 9},
					valueEnd:   Position{Line: 3, Column: 12},
				},
			},
		},
		{
			name:     "unquoted attribute",
			template: `<div class=foo></div>`,
			want: map[string]struct {
				nameStart  Position
				nameEnd    Position
				valueStart Position
				valueEnd   Position
			}{
				"class": {
					nameStart:  Position{Line: 1, Column: 6},
					nameEnd:    Position{Line: 1, Column: 11},
					valueStart: Position{Line: 1, Column: 12},
					valueEnd:   Position{Line: 1, Column: 15},
				},
			},
		},
		{
			name:     "single quotes",
			template: `<div class='foo'></div>`,
			want: map[string]struct {
				nameStart  Position
				nameEnd    Position
				valueStart Position
				valueEnd   Position
			}{
				"class": {
					nameStart:  Position{Line: 1, Column: 6},
					nameEnd:    Position{Line: 1, Column: 11},
					valueStart: Position{Line: 1, Column: 13},
					valueEnd:   Position{Line: 1, Column: 16},
				},
			},
		},
		{
			name:     "empty attribute value",
			template: `<div class=""></div>`,
			want: map[string]struct {
				nameStart  Position
				nameEnd    Position
				valueStart Position
				valueEnd   Position
			}{
				"class": {
					nameStart:  Position{Line: 1, Column: 6},
					nameEnd:    Position{Line: 1, Column: 11},
					valueStart: Position{Line: 1, Column: 13},
					valueEnd:   Position{Line: 1, Column: 13},
				},
			},
		},
		{
			name:     "boolean attribute",
			template: `<input disabled></input>`, // Added closing tag
			want: map[string]struct {
				nameStart  Position
				nameEnd    Position
				valueStart Position
				valueEnd   Position
			}{
				"disabled": {
					nameStart: Position{Line: 1, Column: 8},
					nameEnd:   Position{Line: 1, Column: 16},
				},
			},
		},
		{
			name: "attributes with spaces",
			template: `<div class = "foo"
    id  =  "bar"></div>`,
			want: map[string]struct {
				nameStart  Position
				nameEnd    Position
				valueStart Position
				valueEnd   Position
			}{
				"class": {
					nameStart:  Position{Line: 1, Column: 6},
					nameEnd:    Position{Line: 1, Column: 11},
					valueStart: Position{Line: 1, Column: 15},
					valueEnd:   Position{Line: 1, Column: 18},
				},
				"id": {
					nameStart:  Position{Line: 2, Column: 5},
					nameEnd:    Position{Line: 2, Column: 7},
					valueStart: Position{Line: 2, Column: 13},
					valueEnd:   Position{Line: 2, Column: 16},
				},
			},
		},
		{
			name:     "mixed quote styles",
			template: `<div class="foo" id='bar' data=baz></div>`,
			want: map[string]struct {
				nameStart  Position
				nameEnd    Position
				valueStart Position
				valueEnd   Position
			}{
				"class": {
					nameStart:  Position{Line: 1, Column: 6},
					nameEnd:    Position{Line: 1, Column: 11},
					valueStart: Position{Line: 1, Column: 13},
					valueEnd:   Position{Line: 1, Column: 16},
				},
				"id": {
					nameStart:  Position{Line: 1, Column: 18},
					nameEnd:    Position{Line: 1, Column: 20},
					valueStart: Position{Line: 1, Column: 22},
					valueEnd:   Position{Line: 1, Column: 25},
				},
				"data": {
					nameStart:  Position{Line: 1, Column: 27},
					nameEnd:    Position{Line: 1, Column: 31},
					valueStart: Position{Line: 1, Column: 32},
					valueEnd:   Position{Line: 1, Column: 35},
				},
			},
		},
		{
			name:     "value with spaces",
			template: `<div class="foo bar"></div>`,
			want: map[string]struct {
				nameStart  Position
				nameEnd    Position
				valueStart Position
				valueEnd   Position
			}{
				"class": {
					nameStart:  Position{Line: 1, Column: 6},
					nameEnd:    Position{Line: 1, Column: 11},
					valueStart: Position{Line: 1, Column: 13},
					valueEnd:   Position{Line: 1, Column: 20},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.template)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			// Find the first non-root element
			var node *html.Node
			for c := result.Root.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode {
					node = c
					break
				}
			}

			if node == nil {
				t.Fatal("No element node found")
			}

			pos, ok := result.NodePositions[node]
			if !ok {
				t.Fatal("No position information found for node")
			}

			if pos.Attributes == nil {
				t.Fatal("No attribute positions found")
			}

			for attrName, want := range tt.want {
				attrPos, ok := pos.Attributes[attrName]
				if !ok {
					t.Errorf("Attribute position not found for %q", attrName)
					continue
				}

				if attrPos.NameStart != want.nameStart {
					t.Errorf("Attribute %q NameStart = %v, want %v", attrName, attrPos.NameStart, want.nameStart)
				}
				if attrPos.NameEnd != want.nameEnd {
					t.Errorf("Attribute %q NameEnd = %v, want %v", attrName, attrPos.NameEnd, want.nameEnd)
				}
				if attrPos.ValueStart != want.valueStart {
					t.Errorf("Attribute %q ValueStart = %v, want %v", attrName, attrPos.ValueStart, want.valueStart)
				}
				if attrPos.ValueEnd != want.valueEnd {
					t.Errorf("Attribute %q ValueEnd = %v, want %v", attrName, attrPos.ValueEnd, want.valueEnd)
				}
			}
		})
	}
}
