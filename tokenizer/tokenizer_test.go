package tokenizer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"
)

// formatTokenType converts a TokenType to its string representation
func formatTokenType(t TokenType) string {
	switch t {
	case Doctype:
		return "Doctype"
	case StartTag:
		return "StartTag"
	case EndTag:
		return "EndTag"
	case SelfClosingTag:
		return "SelfClosingTag"
	case Text:
		return "Text"
	case Comment:
		return "Comment"
	case Error:
		return "Error"
	default:
		return "Unknown"
	}
}

// formatToken formats a token for comparison with expected output
func formatToken(t Token) string {
	switch t.Type {
	case Text, Doctype, Comment:
		return fmt.Sprintf("%s %d:%d-%d:%d",
			formatTokenType(t.Type),
			t.Start.Line, t.Start.Column,
			t.End.Line, t.End.Column)
	case StartTag, EndTag, SelfClosingTag:
		return fmt.Sprintf("%s(%s) %d:%d-%d:%d",
			formatTokenType(t.Type), t.Value,
			t.Start.Line, t.Start.Column,
			t.End.Line, t.End.Column)
	case Error:
		return formatTokenType(t.Type)
	default:
		return formatTokenType(t.Type)
	}
}

// parseTestFile parses a txtar file and returns the input HTML and expected tokens
func parseTestFile(filename string) (string, []string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", nil, err
	}

	archive := txtar.Parse(data)

	var inputHTML string
	var expectedTokens []string

	for _, file := range archive.Files {
		switch file.Name {
		case "input.html":
			inputHTML = strings.TrimSpace(string(file.Data))
		case "tokens.txt":
			tokenLines := strings.TrimSpace(string(file.Data))
			if tokenLines != "" {
				expectedTokens = strings.Split(tokenLines, "\n")
			}
		}
	}

	return inputHTML, expectedTokens, nil
}

// TestTokenizerExamples runs all tokenizer tests from txtar files
func TestTokenizerExamples(t *testing.T) {
	testDataDir := "test_data"

	// Check if test_data directory exists
	if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
		t.Skipf("Test data directory %s does not exist", testDataDir)
		return
	}

	// Read all files in the test_data directory
	err := filepath.WalkDir(testDataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-txtar files
		if d.IsDir() || !strings.HasSuffix(path, ".txtar") {
			return nil
		}

		// Get relative path for test name
		relPath, err := filepath.Rel(testDataDir, path)
		if err != nil {
			relPath = filepath.Base(path)
		}

		t.Run(relPath, func(t *testing.T) {
			// Parse the test file
			inputHTML, expectedTokens, err := parseTestFile(path)
			if err != nil {
				t.Fatalf("Failed to parse test file %s: %v", path, err)
			}

			// Tokenize the input
			tokenizer := NewTokenizer(inputHTML)
			tokens := tokenizer.Tokenize()

			// Format the actual tokens
			actualTokens := make([]string, len(tokens))
			for i, token := range tokens {
				actualTokens[i] = formatToken(token)
			}

			// Compare the string slices directly (like TypeScript .toEqual())
			if !reflect.DeepEqual(actualTokens, expectedTokens) {
				t.Errorf("Token mismatch:\nExpected:\n%s\n\nActual:\n%s",
					strings.Join(expectedTokens, "\n"),
					strings.Join(actualTokens, "\n"))
			}
		})

		return nil
	})
	if err != nil {
		t.Fatalf("Failed to walk test directory: %v", err)
	}
}
