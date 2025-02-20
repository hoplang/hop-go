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
	var f func(*html.Node, *html.Node) bool
	f = func(a *html.Node, b *html.Node) bool {
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
			res := f(ac, bc)
			if !res {
				return res
			}
			ac = ac.NextSibling
			bc = bc.NextSibling
		}
		return true
	}
	doc, _ := html.Parse(strings.NewReader(a))
	doc2, _ := html.Parse(strings.NewReader(b))
	return f(doc, doc2)
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
	templateData := findFile("template")
	if templateData == nil {
		t.Fatal("Failed to extract template data")
	}
	expectedError := strings.TrimSpace(string(findFile("error")))
	if expectedError == "" {
		t.Fatal("Failed to extract expected error")
	}

	_, err = hop.Parse(string(templateData))
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
	jsonData := findFile("json")
	if jsonData == nil {
		t.Fatal("Failed to extract JSON data")
	}
	templateData := findFile("template")
	if templateData == nil {
		t.Fatal("Failed to extract template data")
	}
	expectedError := strings.TrimSpace(string(findFile("error")))
	if expectedError == "" {
		t.Fatal("Failed to extract expected error")
	}

	var d any
	err = json.Unmarshal(jsonData, &d)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %s", err)
	}

	var buf bytes.Buffer

	tmpl, err := hop.Parse(string(templateData))
	if err != nil {
		t.Fatalf("Expected runtime error but got parse error: %s", err.Error())
		return
	}

	err = tmpl.ExecuteFunction(&buf, "main", d)
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
	jsonData := findFile("json")
	if jsonData == nil {
		t.Fatal("Failed to extract JSON data")
	}
	templateData := findFile("template")
	if templateData == nil {
		t.Fatal("Failed to extract template data")
	}
	expectedHTML := findFile("html")
	if expectedHTML == nil {
		t.Fatal("Failed to extract expected HTML")
	}

	var d any
	err = json.Unmarshal(jsonData, &d)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %s", err)
	}

	var buf bytes.Buffer

	tmpl, err := hop.Parse(string(templateData))
	if err != nil {
		t.Errorf("Failed to parse template: %s", err)
	}

	err = tmpl.ExecuteFunction(&buf, "main", d)
	if err != nil {
		t.Errorf("Failed to execute template: %s", err)
	}
	equal := compareHTML(strings.TrimSpace(string(expectedHTML)), strings.TrimSpace(buf.String()))
	if !equal {
		t.Errorf("Template processing failed.\nExpected:\n%s\nGot:\n%s",
			expectedHTML, buf.String())
	}
}
