package hop_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoplang/hop-go"
	"github.com/hoplang/hop-go/parser"
	"golang.org/x/net/html"
	"golang.org/x/tools/txtar"
)

func TestTemplates(t *testing.T) {
	entries, err := os.ReadDir("test_data/runtime_outputs")
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".txtar") {
			t.Run(entry.Name(), func(t *testing.T) {
				testFile(t, filepath.Join("test_data/runtime_outputs", entry.Name()))
			})
		}
	}
}

func TestRuntimeErrors(t *testing.T) {
	entries, err := os.ReadDir("test_data/runtime_errors")
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".txtar") {
			t.Run(entry.Name(), func(t *testing.T) {
				testRuntimeError(t, filepath.Join("test_data/runtime_errors", entry.Name()))
			})
		}
	}
}

func TestParseErrors(t *testing.T) {
	entries, err := os.ReadDir("test_data/parse_errors")
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".txtar") {
			t.Run(entry.Name(), func(t *testing.T) {
				testParseError(t, filepath.Join("test_data/parse_errors", entry.Name()))
			})
		}
	}
}

func TestTypeErrors(t *testing.T) {
	entries, err := os.ReadDir("test_data/type_errors")
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".txtar") {
			t.Run(entry.Name(), func(t *testing.T) {
				testTypeError(t, filepath.Join("test_data/type_errors", entry.Name()))
			})
		}
	}
}

func compareHTML(a string, b string) bool {
	doc, _ := html.Parse(strings.NewReader(a))
	doc2, _ := html.Parse(strings.NewReader(b))
	return compareHTMLNodes(doc, doc2)
}

func compareHTMLNodes(a *html.Node, b *html.Node) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Type != b.Type {
		return false
	}
	if a.Type == html.TextNode {
		if a.Data != b.Data {
			if strings.TrimSpace(a.Data) == "" && strings.TrimSpace(b.Data) == "" {
				// Allow that two text nodes differ in the amount
				// of whitespace if they only consist of whitespace
			} else {
				return false
			}
		}
	} else {
		if a.Data != b.Data {
			return false
		}
	}
	if !reflect.DeepEqual(a.Attr, b.Attr) {
		return false
	}
	ac := a.FirstChild
	bc := b.FirstChild
	for ac != nil || bc != nil {
		if !compareHTMLNodes(ac, bc) {
			return false
		}
		ac = ac.NextSibling
		bc = bc.NextSibling
	}
	return true
}

func testParseError(t *testing.T, filename string) {
	// Read the txtar file from testdata directory
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Parse the txtar file
	archive := txtar.Parse(data)

	// Helper function to find file content by name
	findFile := func(name string) []byte {
		for _, file := range archive.Files {
			if file.Name == name {
				return file.Data
			}
		}
		return nil
	}

	// Extract the components
	templateData := findFile("main.hop")
	if templateData == nil {
		t.Fatal("Failed to extract template data")
	}
	expectedError := strings.TrimSpace(string(findFile("error.txt")))
	if expectedError == "" {
		t.Fatal("Failed to extract expected error")
	}

	_, err = parser.Parse(string(templateData))
	if err == nil {
		t.Fatalf("Expected error to contain '%s' but got nil", expectedError)
	}
	if err != nil {
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s' but got %s",
				expectedError, err.Error())
		}
		return
	}
}

func testTypeError(t *testing.T, filename string) {
	// Read the txtar file from testdata directory
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Parse the txtar file
	archive := txtar.Parse(data)

	// Helper function to find file content by name
	findFile := func(name string) []byte {
		for _, file := range archive.Files {
			if file.Name == name {
				return file.Data
			}
		}
		return nil
	}

	// Extract the components
	templateData := findFile("main.hop")
	if templateData == nil {
		t.Fatal("Failed to extract template data")
	}
	expectedError := strings.TrimSpace(string(findFile("error.txt")))
	if expectedError == "" {
		t.Fatal("Failed to extract expected error")
	}

	p := hop.NewCompiler()
	p.AddModule("main", string(templateData))
	_, err = p.Compile()
	if err == nil {
		t.Fatalf("Expected error to contain '%s' but got nil", expectedError)
	}
	if err != nil {
		if !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("Expected error to contain '%s' but got %s",
				expectedError, err.Error())
		}
		return
	}
}

func testRuntimeError(t *testing.T, filename string) {
	// Read the txtar file from testdata directory
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Parse the txtar file
	archive := txtar.Parse(data)

	// Helper function to find file content by name
	findFile := func(name string) []byte {
		for _, file := range archive.Files {
			if file.Name == name {
				return file.Data
			}
		}
		return nil
	}

	// Extract the components
	jsonData := findFile("data.json")
	if jsonData == nil {
		t.Fatal("Failed to extract JSON data")
	}
	templateData := findFile("main.hop")
	if templateData == nil {
		t.Fatal("Failed to extract template data")
	}
	expectedError := strings.TrimSpace(string(findFile("error.txt")))
	if expectedError == "" {
		t.Fatal("Failed to extract expected error")
	}

	var d any
	err = json.Unmarshal(jsonData, &d)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %s", err)
	}

	var buf bytes.Buffer

	p := hop.NewCompiler()
	p.AddModule("main", string(templateData))
	cp, err := p.Compile()
	if err != nil {
		t.Fatalf("Expected runtime error but got compile error: %s", err.Error())
		return
	}

	err = cp.ExecuteFunction(&buf, "main", "main", d)
	if err == nil {
		t.Errorf("Expected runtime error '%s' but got nil", expectedError)
	}
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected runtime error to contain '%s' but got %s",
			expectedError, err.Error())
	}
}

func testFile(t *testing.T, filename string) {
	// Read the txtar file from testdata directory
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Parse the txtar file
	archive := txtar.Parse(data)

	// Helper function to find file content by name
	findFile := func(name string) []byte {
		for _, file := range archive.Files {
			if file.Name == name {
				return file.Data
			}
		}
		return nil
	}

	// Extract the components
	jsonData := findFile("data.json")
	if jsonData == nil {
		t.Fatal("Failed to extract JSON data")
	}
	expectedHTML := findFile("output.html")
	if expectedHTML == nil {
		t.Fatal("Failed to extract expected HTML")
	}

	var d any
	err = json.Unmarshal(jsonData, &d)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %s", err)
	}

	var buf bytes.Buffer

	p := hop.NewCompiler()
	for _, file := range archive.Files {
		if strings.HasSuffix(file.Name, ".hop") {
			parts := strings.Split(file.Name, ".")
			p.AddModule(parts[0], string(file.Data))
		}
	}
	c, err := p.Compile()
	if err != nil {
		t.Errorf("Failed to compile: %s", err)
	}

	err = c.ExecuteFunction(&buf, "main", "main", d)
	if err != nil {
		t.Errorf("Failed to execute function: %s", err)
	}
	equal := compareHTML(strings.TrimSpace(string(expectedHTML)), strings.TrimSpace(buf.String()))
	if !equal {
		t.Errorf("Expected:\n%s\nGot:\n%s",
			expectedHTML, buf.String())
	}
}
