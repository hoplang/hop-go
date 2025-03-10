package parser

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var validAttrNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9\-_]*$`)

var voidElements = map[string]bool{
	"area":   true,
	"base":   true,
	"br":     true,
	"col":    true,
	"embed":  true,
	"hr":     true,
	"img":    true,
	"input":  true,
	"link":   true,
	"meta":   true,
	"param":  true,
	"source": true,
	"track":  true,
	"wbr":    true,
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

		case html.DoctypeToken:
			// Handle doctype declaration
			node := &html.Node{
				Type: html.DoctypeNode,
				Data: token.Data,
			}
			parent := stack[len(stack)-1]
			parent.AppendChild(node)

			// Store position information for the doctype node
			result.NodePositions[node] = NodePosition{
				Start: pos,
				End:   tokenizer.pos,
			}

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

			attrPositions := tokenizer.parseAttributePositions(
				tokenizer.tokenizer.Raw(),
				pos,
			)

			// Store node position with attributes
			nodePos := NodePosition{
				Start:      pos,
				Attributes: attrPositions,
			}

			// For void elements, set the end position immediately
			if voidElements[token.Data] {
				nodePos.End = tokenizer.pos
			}
			result.NodePositions[node] = nodePos

			parent := stack[len(stack)-1]
			parent.AppendChild(node)

			// Only push non-void elements onto the stack
			if !voidElements[token.Data] {
				stack = append(stack, node)
			}

		case html.EndTagToken:
			// Ignore end tags for void elements
			if voidElements[token.Data] {
				continue
			}

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

		case html.CommentToken:
			// Handle comment nodes
			node := &html.Node{
				Type: html.CommentNode,
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
