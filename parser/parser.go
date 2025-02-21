package parser

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var validAttrNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9\-_]*$`)

type ParseError struct {
	Pos     Position
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Message)
}

type ParseResult struct {
	Root          *html.Node
	NodePositions map[*html.Node]NodePosition
}

type Position struct {
	Line   int
	Column int
}

func (p Position) String() string {
	return fmt.Sprintf("line %d, column %d", p.Line, p.Column)
}

type AttributePosition struct {
	NameStart  Position
	NameEnd    Position
	ValueStart Position
	ValueEnd   Position
}

type NodePosition struct {
	Start      Position
	End        Position
	Attributes map[string]AttributePosition
}

type positionTrackingTokenizer struct {
	tokenizer *html.Tokenizer
	pos       Position
	input     string
}

func newPositionTrackingTokenizer(r io.Reader) (*positionTrackingTokenizer, error) {
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &positionTrackingTokenizer{
		tokenizer: html.NewTokenizer(bytes.NewReader(input)),
		pos:       Position{Line: 1, Column: 1},
		input:     string(input),
	}, nil
}

func (t *positionTrackingTokenizer) next() (html.TokenType, html.Token, Position) {
	startPos := t.pos
	tt := t.tokenizer.Next()
	tok := t.tokenizer.Token()
	raw := t.tokenizer.Raw()
	for _, c := range raw {
		if c == '\n' {
			t.pos.Line++
			t.pos.Column = 1
		} else {
			t.pos.Column++
		}
	}

	return tt, tok, startPos
}

func (t *positionTrackingTokenizer) parseAttributePositions(raw []byte, startPos Position) map[string]AttributePosition {
	attrPositions := make(map[string]AttributePosition)
	pos := startPos
	i := 0

	// Skip opening < and tag name
	for i < len(raw) && !isWhitespace(raw[i]) && raw[i] != '>' {
		t.advancePosition(&pos, raw[i])
		i++
	}

	// Parse attributes
	for i < len(raw) {
		// Skip whitespace
		for i < len(raw) && isWhitespace(raw[i]) {
			t.advancePosition(&pos, raw[i])
			i++
		}

		if i >= len(raw) || raw[i] == '>' || raw[i] == '/' {
			break
		}

		// Parse attribute name
		nameStart := pos
		nameEnd := pos
		nameStartIndex := i

		for i < len(raw) && !isWhitespace(raw[i]) && raw[i] != '=' && raw[i] != '>' && raw[i] != '/' {
			t.advancePosition(&pos, raw[i])
			nameEnd = pos
			i++
		}

		// Extract name from raw bytes directly
		name := string(raw[nameStartIndex:i])

		attrPos := AttributePosition{
			NameStart: nameStart,
			NameEnd:   nameEnd,
		}

		// Skip whitespace before potential =
		for i < len(raw) && isWhitespace(raw[i]) {
			t.advancePosition(&pos, raw[i])
			i++
		}

		// Handle = case
		if i < len(raw) && raw[i] == '=' {
			t.advancePosition(&pos, raw[i])
			i++

			// Skip whitespace after =
			for i < len(raw) && isWhitespace(raw[i]) {
				t.advancePosition(&pos, raw[i])
				i++
			}

			// Handle empty value case
			if raw[i] == '"' || raw[i] == '\'' {
				// Handle quoted value
				quote := raw[i]
				t.advancePosition(&pos, raw[i])
				i++

				valueStart := pos
				valueEnd := pos

				for i < len(raw) && raw[i] != quote {
					t.advancePosition(&pos, raw[i])
					valueEnd = pos
					i++
				}

				attrPos.ValueStart = valueStart
				attrPos.ValueEnd = valueEnd

				if i < len(raw) && raw[i] == quote {
					t.advancePosition(&pos, raw[i])
					i++
				}
			} else {
				// Handle unquoted value
				valueStart := pos
				for i < len(raw) && !isWhitespace(raw[i]) && raw[i] != '>' && raw[i] != '/' {
					t.advancePosition(&pos, raw[i])
					i++
				}
				attrPos.ValueStart = valueStart
				attrPos.ValueEnd = pos
			}
		}

		attrPositions[name] = attrPos
	}

	return attrPositions
}

func (t *positionTrackingTokenizer) advancePosition(pos *Position, c byte) {
	if c == '\n' {
		pos.Line++
		pos.Column = 1
	} else {
		pos.Column++
	}
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func newParseError(pos Position, format string, args ...interface{}) *ParseError {
	return &ParseError{
		Pos:     pos,
		Message: "parse error: " + fmt.Sprintf(format, args...),
	}
}

func Parse(template string) (*ParseResult, error) {
	result := &ParseResult{
		NodePositions: make(map[*html.Node]NodePosition),
	}

	root := &html.Node{
		Type: html.ElementNode,
		Data: "root",
	}
	result.Root = root

	tokenizer, err := newPositionTrackingTokenizer(strings.NewReader(template))
	if err != nil {
		return nil, err
	}

	var stack []*html.Node
	stack = append(stack, root)
	startPositions := make(map[*html.Node]Position)

	for {
		tokenType, token, pos := tokenizer.next()

		switch tokenType {
		case html.ErrorToken:
			if len(stack) > 1 {
				lastNode := stack[len(stack)-1]
				startPos := startPositions[lastNode]
				return nil, newParseError(startPos, "unclosed tag <%s>", lastNode.Data)
			}
			return result, nil

		case html.StartTagToken:
			// Validate attributes before creating the node
			for _, attr := range token.Attr {
				if attr.Key == "" || !validAttrNameRegex.MatchString(attr.Key) {
					return nil, newParseError(pos, "invalid attribute: %v", attr.Key)
				}
			}

			node := &html.Node{
				Type:     html.ElementNode,
				Data:     token.Data,
				DataAtom: token.DataAtom,
				Attr:     token.Attr,
			}
			startPositions[node] = pos

			// Parse attribute positions from raw token
			raw := tokenizer.tokenizer.Raw()
			attrPositions := tokenizer.parseAttributePositions(raw, pos)

			// Store node position with attributes
			result.NodePositions[node] = NodePosition{
				Start:      pos,
				Attributes: attrPositions,
			}

			parent := stack[len(stack)-1]
			parent.AppendChild(node)
			stack = append(stack, node)

		case html.EndTagToken:
			if len(stack) == 0 {
				return nil, newParseError(pos, "unexpected closing tag </%s>", token.Data)
			}

			node := stack[len(stack)-1]
			if node.Data != token.Data {
				return nil, newParseError(pos, "mismatched closing tag: expected </%s>, got </%s>",
					node.Data, token.Data)
			}

			stack = stack[:len(stack)-1]
			nodePos := result.NodePositions[node]
			nodePos.End = pos
			result.NodePositions[node] = nodePos

		case html.TextToken:
			node := &html.Node{
				Type: html.TextNode,
				Data: token.Data,
			}
			parent := stack[len(stack)-1]
			parent.AppendChild(node)
			result.NodePositions[node] = NodePosition{
				Start: pos,
				End:   tokenizer.pos,
			}
		}
	}
}
