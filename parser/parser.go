package parser

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
)

type Position struct {
	Line   int
	Column int
	Offset int
}

func (p Position) String() string {
	return fmt.Sprintf("line %d, column %d", p.Line, p.Column)
}

type NodePosition struct {
	Start Position
	End   Position
}

type ParseError struct {
	Pos     Position
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Message)
}

type ParseResult struct {
	Root          *html.Node
	NodePositions map[*html.Node]*NodePosition
}

func NewParseResult() *ParseResult {
	return &ParseResult{
		NodePositions: make(map[*html.Node]*NodePosition),
	}
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

func (t *positionTrackingTokenizer) Next() (html.TokenType, html.Token, Position) {
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
		t.pos.Offset++
	}

	return tt, tok, startPos
}

func newParseError(pos Position, format string, args ...interface{}) *ParseError {
	return &ParseError{
		Pos:     pos,
		Message: fmt.Sprintf(format, args...),
	}
}

func Parse(template string) (*ParseResult, error) {
	result := NewParseResult()

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
		tokenType, token, pos := tokenizer.Next()

		switch tokenType {
		case html.ErrorToken:
			if len(stack) > 1 {
				lastNode := stack[len(stack)-1]
				startPos := startPositions[lastNode]
				return nil, newParseError(startPos, "unclosed tag <%s>", lastNode.Data)
			}
			return result, nil

		case html.StartTagToken:
			node := &html.Node{
				Type:     html.ElementNode,
				Data:     token.Data,
				DataAtom: token.DataAtom,
				Attr:     token.Attr,
			}
			startPositions[node] = pos
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
			result.NodePositions[node] = &NodePosition{
				Start: startPositions[node],
				End:   pos,
			}

		case html.TextToken:
			// Create text node without trimming whitespace
			node := &html.Node{
				Type: html.TextNode,
				Data: token.Data,
			}
			parent := stack[len(stack)-1]
			parent.AppendChild(node)
			result.NodePositions[node] = &NodePosition{
				Start: pos,
				End:   tokenizer.pos,
			}
		}
	}
}
