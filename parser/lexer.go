package parser

import (
	"io"

	"golang.org/x/net/html"
)

type positionTrackingTokenizer struct {
	tokenizer *html.Tokenizer
	pos       Position
}

func newPositionTrackingTokenizer(r io.Reader) (*positionTrackingTokenizer, error) {
	return &positionTrackingTokenizer{
		tokenizer: html.NewTokenizer(r),
		pos:       Position{Line: 1, Column: 1},
	}, nil
}

func (t *positionTrackingTokenizer) next() (html.TokenType, html.Token, Position) {
	startPos := t.pos
	tt := t.tokenizer.Next()
	tok := t.tokenizer.Token()
	raw := t.tokenizer.Raw()
	for _, c := range raw {
		t.advancePosition(&t.pos, c)
	}

	return tt, tok, startPos
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
