package tokenizer

import (
	"strings"
	"unicode"
)

// TokenizerState represents the current state of the tokenizer
type TokenizerState int

const (
	TEXT TokenizerState = iota
	TAG_OPEN
	START_TAG_NAME
	END_TAG_OPEN
	END_TAG_NAME
	AFTER_END_TAG_NAME
	BEFORE_ATTR_NAME
	ATTR_NAME
	AFTER_ATTR_NAME
	BEFORE_ATTR_VALUE
	ATTR_VALUE_DOUBLE_QUOTE
	ATTR_VALUE_SINGLE_QUOTE
	SELF_CLOSING
	MARKUP_DECLARATION
	COMMENT
	DOCTYPE
	BEFORE_DOCTYPE_NAME
	DOCTYPE_NAME
	RAWTEXT_DATA
)

// TokenType represents the type of a token
type TokenType int

const (
	Doctype TokenType = iota
	StartTag
	EndTag
	SelfClosingTag
	Text
	Comment
	Error
)

// Position represents a position in the source code
type Position struct {
	Line   int
	Column int
}

// Attribute represents an HTML attribute
type Attribute struct {
	Name  string
	Value string
	Start Position
	End   Position
}

// Token represents a lexical token
type Token struct {
	Type       TokenType
	Value      string
	Attributes []Attribute
	Start      Position
	End        Position
}

// Tokenizer tokenizes Hop language source code
type Tokenizer struct {
	input            string
	state            TokenizerState
	position         Position
	currentPosition  int
	tokens           []Token
	currentToken     *Token
	currentAttribute *Attribute

	doctypeNameBuffer string
	storedTagName     string

	// Special tag names that trigger RAWTEXT_DATA state
	specialTagNames map[string]bool
}

// NewTokenizer creates a new tokenizer with the given input
func NewTokenizer(input string) *Tokenizer {
	return &Tokenizer{
		input:           input,
		state:           TEXT,
		position:        Position{Line: 1, Column: 1},
		currentPosition: 0,
		tokens:          make([]Token, 0),
		specialTagNames: map[string]bool{
			"textarea": true,
			"title":    true,
			"script":   true,
			"style":    true,
			"template": true,
		},
	}
}

// peek returns the next character without consuming it
func (t *Tokenizer) peek() rune {
	if t.currentPosition >= len(t.input) {
		return 0
	}
	return rune(t.input[t.currentPosition])
}

// advance consumes the next character and advances the position
func (t *Tokenizer) advance() rune {
	if t.currentPosition >= len(t.input) {
		return 0
	}
	char := rune(t.input[t.currentPosition])
	t.currentPosition++
	if char == '\n' {
		t.position.Line++
		t.position.Column = 1
	} else {
		t.position.Column++
	}
	return char
}

// initializeToken creates a new current token
func (t *Tokenizer) initializeToken() {
	t.currentToken = &Token{
		Type:       Text,
		Value:      "",
		Attributes: make([]Attribute, 0),
		Start:      t.position,
		End:        t.position,
	}
}

// pushCurrentToken adds the current token to the tokens slice
func (t *Tokenizer) pushCurrentToken() {
	if t.currentToken == nil {
		panic("Expected current token to be defined when pushing current token")
	}
	t.currentToken.End = t.position
	t.tokens = append(t.tokens, *t.currentToken)
	t.currentToken = nil
}

// initializeAttribute creates a new current attribute
func (t *Tokenizer) initializeAttribute() {
	t.currentAttribute = &Attribute{
		Name:  "",
		Value: "",
		Start: t.position,
		End:   t.position,
	}
}

// pushCurrentAttribute adds the current attribute to the current token
func (t *Tokenizer) pushCurrentAttribute() {
	if t.currentToken == nil {
		panic("Expected current token to be defined when pushing current attribute")
	}
	if t.currentAttribute == nil {
		panic("Expected current attribute to be defined when pushing current attribute")
	}
	t.currentAttribute.End = t.position
	t.currentToken.Attributes = append(t.currentToken.Attributes, *t.currentAttribute)
	t.currentAttribute = nil
}

// pushErrorToken creates and pushes an error token
func (t *Tokenizer) pushErrorToken(message string) {
	if t.currentToken == nil {
		t.initializeToken()
	}
	t.currentToken.Type = Error
	t.currentToken.Value = message
	t.pushCurrentToken()
	t.state = TEXT
}

// isLetter checks if a character is a letter
func isLetter(char rune) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

// isAlphanumeric checks if a character is alphanumeric or hyphen
func isAlphanumeric(char rune) bool {
	return isLetter(char) || (char >= '0' && char <= '9') || char == '-'
}

// isWhitespace checks if a character is whitespace
func isWhitespace(char rune) bool {
	return unicode.IsSpace(char)
}

// checkSpecialTag checks if the current tag name is a special tag
func (t *Tokenizer) checkSpecialTag() bool {
	if t.currentToken == nil {
		return false
	}
	return t.specialTagNames[strings.ToLower(t.currentToken.Value)]
}

// checkDoctypeString checks if the next characters match "DOCTYPE"
func (t *Tokenizer) checkDoctypeString() bool {
	remaining := t.input[t.currentPosition:]
	return strings.HasPrefix(strings.ToUpper(remaining), "DOCTYPE")
}

// checkCommentStart checks if the next characters match "--"
func (t *Tokenizer) checkCommentStart() bool {
	remaining := t.input[t.currentPosition:]
	return strings.HasPrefix(remaining, "--")
}

// checkCommentEnd checks if the next characters match "-->"
func (t *Tokenizer) checkCommentEnd() bool {
	remaining := t.input[t.currentPosition:]
	return strings.HasPrefix(remaining, "-->")
}

// checkEndTag checks if the current position matches the stored tag name for rawtext
func (t *Tokenizer) checkEndTag() bool {
	remaining := t.input[t.currentPosition:]
	expected := "</" + t.storedTagName + ">"
	return strings.HasPrefix(strings.ToLower(remaining), strings.ToLower(expected))
}

// Tokenize processes the input and returns the tokens
func (t *Tokenizer) Tokenize() []Token {
	// Initialize with a text token
	t.initializeToken()

	for t.currentPosition < len(t.input) {
		char := t.peek()

		switch t.state {
		case TEXT:
			if char == '<' {
				// Push current token if it has content
				if t.currentToken != nil && t.currentToken.Value != "" {
					t.pushCurrentToken()
				}
				// Initialize new token before advancing
				t.initializeToken()
				t.advance()
				t.state = TAG_OPEN
			} else {
				if t.currentToken == nil {
					t.initializeToken()
				}
				t.currentToken.Value += string(t.advance())
			}

		case TAG_OPEN:
			if isLetter(char) {
				t.currentToken.Type = StartTag
				t.currentToken.Value += string(t.advance())
				t.state = START_TAG_NAME
			} else if char == '/' {
				t.currentToken.Type = EndTag
				t.advance()
				t.state = END_TAG_OPEN
			} else if char == '!' {
				t.advance()
				t.state = MARKUP_DECLARATION
			} else {
				t.advance()
				t.pushErrorToken("Invalid character after '<'")
			}

		case START_TAG_NAME:
			if isAlphanumeric(char) {
				t.currentToken.Value += string(t.advance())
			} else if isWhitespace(char) {
				t.advance()
				t.state = BEFORE_ATTR_NAME
			} else if char == '>' {
				t.advance()
				if t.checkSpecialTag() {
					t.storedTagName = t.currentToken.Value
					t.pushCurrentToken()
					t.initializeToken()
					t.state = RAWTEXT_DATA
				} else {
					t.pushCurrentToken()
					t.initializeToken()
					t.state = TEXT
				}
			} else if char == '/' {
				t.currentToken.Type = SelfClosingTag
				t.advance()
				t.state = SELF_CLOSING
			} else {
				t.advance()
				t.pushErrorToken("Invalid character in tag name")
			}

		case END_TAG_OPEN:
			if isLetter(char) {
				t.currentToken.Value += string(t.advance())
				t.state = END_TAG_NAME
			} else {
				t.advance()
				t.pushErrorToken("Expected tag name after '</'")
			}

		case END_TAG_NAME:
			if isAlphanumeric(char) {
				t.currentToken.Value += string(t.advance())
			} else if char == '>' {
				t.advance()
				t.pushCurrentToken()
				t.initializeToken()
				t.state = TEXT
			} else if isWhitespace(char) {
				t.advance()
				t.state = AFTER_END_TAG_NAME
			} else {
				t.advance()
				t.pushErrorToken("Invalid character in end tag name")
			}

		case AFTER_END_TAG_NAME:
			if isWhitespace(char) {
				t.advance()
			} else if char == '>' {
				t.advance()
				t.pushCurrentToken()
				t.initializeToken()
				t.state = TEXT
			} else {
				t.advance()
				t.pushErrorToken("Expected '>' after end tag name")
			}

		case BEFORE_ATTR_NAME:
			if isWhitespace(char) {
				t.advance()
			} else if isLetter(char) {
				t.initializeAttribute()
				t.currentAttribute.Name += string(t.advance())
				t.state = ATTR_NAME
			} else if char == '/' {
				t.currentToken.Type = SelfClosingTag
				t.advance()
				t.state = SELF_CLOSING
			} else if char == '>' {
				t.advance()
				if t.checkSpecialTag() {
					t.storedTagName = t.currentToken.Value
					t.pushCurrentToken()
					t.initializeToken()
					t.state = RAWTEXT_DATA
				} else {
					t.pushCurrentToken()
					t.initializeToken()
					t.state = TEXT
				}
			} else {
				t.advance()
				t.pushErrorToken("Invalid character before attribute name")
			}

		case ATTR_NAME:
			if isLetter(char) || char == '-' {
				t.currentAttribute.Name += string(t.advance())
			} else if isWhitespace(char) {
				t.advance()
				t.state = AFTER_ATTR_NAME
			} else if char == '=' {
				t.advance()
				t.state = BEFORE_ATTR_VALUE
			} else if char == '>' {
				t.pushCurrentAttribute()
				t.advance()
				if t.checkSpecialTag() {
					t.storedTagName = t.currentToken.Value
					t.pushCurrentToken()
					t.initializeToken()
					t.state = RAWTEXT_DATA
				} else {
					t.pushCurrentToken()
					t.initializeToken()
					t.state = TEXT
				}
			} else if char == '/' {
				t.pushCurrentAttribute()
				t.currentToken.Type = SelfClosingTag
				t.advance()
				t.state = SELF_CLOSING
			} else {
				t.advance()
				t.pushErrorToken("Invalid character in attribute name")
			}

		case AFTER_ATTR_NAME:
			if isWhitespace(char) {
				t.advance()
			} else if char == '=' {
				t.advance()
				t.state = BEFORE_ATTR_VALUE
			} else {
				t.advance()
				t.pushErrorToken("Expected '=' after attribute name")
			}

		case BEFORE_ATTR_VALUE:
			if isWhitespace(char) {
				t.advance()
			} else if char == '"' {
				t.advance()
				t.state = ATTR_VALUE_DOUBLE_QUOTE
			} else if char == '\'' {
				t.advance()
				t.state = ATTR_VALUE_SINGLE_QUOTE
			} else {
				t.advance()
				t.pushErrorToken("Expected quoted attribute value")
			}

		case ATTR_VALUE_DOUBLE_QUOTE:
			if char == '"' {
				t.advance()
				t.pushCurrentAttribute()
				t.state = BEFORE_ATTR_NAME
			} else {
				t.currentAttribute.Value += string(t.advance())
			}

		case ATTR_VALUE_SINGLE_QUOTE:
			if char == '\'' {
				t.advance()
				t.pushCurrentAttribute()
				t.state = BEFORE_ATTR_NAME
			} else {
				t.currentAttribute.Value += string(t.advance())
			}

		case SELF_CLOSING:
			if char == '>' {
				t.advance()
				t.pushCurrentToken()
				t.initializeToken()
				t.state = TEXT
			} else {
				t.advance()
				t.pushErrorToken("Expected '>' after '/")
			}

		case MARKUP_DECLARATION:
			if t.checkCommentStart() {
				t.currentToken.Type = Comment
				t.advance() // consume first '-'
				t.advance() // consume second '-'
				t.state = COMMENT
			} else if t.checkDoctypeString() {
				t.currentToken.Type = Doctype
				// Consume "DOCTYPE"
				for i := 0; i < 7; i++ {
					t.advance()
				}
				t.state = DOCTYPE
			} else {
				t.advance()
				t.pushErrorToken("Invalid markup declaration")
			}

		case COMMENT:
			if t.checkCommentEnd() {
				t.advance() // consume first '-'
				t.advance() // consume second '-'
				t.advance() // consume '>'
				t.pushCurrentToken()
				t.initializeToken()
				t.state = TEXT
			} else {
				t.advance()
			}

		case DOCTYPE:
			if isWhitespace(char) {
				t.advance()
				t.state = BEFORE_DOCTYPE_NAME
			} else {
				t.advance()
				t.pushErrorToken("Expected whitespace after DOCTYPE")
			}

		case BEFORE_DOCTYPE_NAME:
			if isWhitespace(char) {
				t.advance()
			} else if isLetter(char) {
				t.doctypeNameBuffer = ""
				t.doctypeNameBuffer += string(t.advance())
				t.state = DOCTYPE_NAME
			} else {
				t.advance()
				t.pushErrorToken("Expected DOCTYPE name")
			}

		case DOCTYPE_NAME:
			if isLetter(char) {
				t.doctypeNameBuffer += string(t.advance())
			} else if char == '>' {
				if strings.ToLower(t.doctypeNameBuffer) == "html" {
					t.advance()
					t.pushCurrentToken()
					t.initializeToken()
					t.state = TEXT
				} else {
					t.advance()
					t.pushErrorToken("Invalid DOCTYPE name")
				}
			} else {
				t.advance()
				t.pushErrorToken("Invalid character in DOCTYPE name")
			}

		case RAWTEXT_DATA:
			if t.checkEndTag() {
				// Push current text token if it has content
				if t.currentToken != nil && t.currentToken.Value != "" {
					t.pushCurrentToken()
				}

				// Create and push end tag token
				endTagStart := t.position
				tagLength := len(t.storedTagName) + 3 // "</" + tagName + ">"
				for i := 0; i < tagLength; i++ {
					t.advance()
				}
				endTagToken := Token{
					Type:       EndTag,
					Value:      t.storedTagName,
					Attributes: make([]Attribute, 0),
					Start:      endTagStart,
					End:        t.position,
				}
				t.tokens = append(t.tokens, endTagToken)

				// Initialize new text token and return to TEXT state
				t.initializeToken()
				t.state = TEXT
			} else {
				if t.currentToken == nil {
					t.initializeToken()
				}
				t.currentToken.Value += string(t.advance())
			}

		default:
			t.advance()
			t.pushErrorToken("Unknown tokenizer state")
		}
	}

	// Push final token if it exists and has content
	if t.currentToken != nil && t.currentToken.Value != "" {
		t.pushCurrentToken()
	}

	return t.tokens
}
