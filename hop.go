package hop

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type Template struct {
	root      *html.Node
	functions map[string]*html.Node
}

type Function struct {
	children []*html.Node
	name     string
}

// Parse takes a template string, parses it and returns
// a template or fails.
func Parse(template string) (*Template, error) {
	root := &html.Node{
		Type: html.ElementNode,
		Data: "root",
	}
	nodes, err := html.ParseFragment(strings.NewReader(template), root)
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	// Collect functions
	functions := map[string]*html.Node{}
	for c := range root.ChildNodes() {
		if c.Type == html.ElementNode && c.Data == "function" {
			var name string
			for _, attr := range c.Attr {
				if attr.Key == "name" {
					name = attr.Val
				}
			}
			if name == "" {
				return nil, fmt.Errorf("function is missing attribute 'name'")
			}
			functions[name] = c
		}
	}
	t := &Template{root: root, functions: functions}
	_, err = t.inferTypes()
	if err != nil {
		return nil, err
	}
	return t, nil
}

func MustParse(template string) *Template {
	t, err := Parse(template)
	if err != nil {
		panic(err)
	}
	return t
}

// ExecuteFunction executes a specific function from the template with the given parameters
func (t *Template) ExecuteFunction(w io.Writer, functionName string, data any) error {
	function, exists := t.functions[functionName]
	if !exists {
		return fmt.Errorf("no function with name %s in scope", functionName)
	}
	functionScope := map[string]any{}
	for _, attr := range function.Attr {
		if attr.Key == "params-as" {
			functionScope[attr.Val] = data
		}
	}
	for c := range function.ChildNodes() {
		nodes, err := t.evaluateNode(c, functionScope)
		if err != nil {
			return err
		}
		for _, n := range nodes {
			err := html.Render(w, n)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func typeof(v any) string {
	switch v.(type) {
	case float64:
		return "number"
	case map[string]any:
		return "object"
	case string:
		return "string"
	case []any:
		return "array"
	default:
		return "invalid"
	}
}

func stringify(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// pathPart represents a part of a path with information
// about whether it's an array index
type pathPart struct {
	value      string
	isArrayRef bool
}

// Match either:
// 1. a segment between dots or at start/end of string
// 2. anything in square brackets
var pathPartRegexp = regexp.MustCompile(`([^.\[\]]+)|\[([^\]]+)\]`)

// parsePath splits a path string into parts.
//
// Examples:
//
//	"foo.bar" => [{foo false} {bar false}]
//	"foo.bar[0].baz" => [{foo false} {bar true} {baz false}]
//	"foo[0][1][2]" => [{foo true} {1 true} {2 true}]
func parsePath(path string) ([]pathPart, error) {
	matches := pathPartRegexp.FindAllStringSubmatch(path, -1)
	components := []pathPart{}
	for _, match := range matches {
		if match[1] != "" {
			components = append(components, pathPart{
				value:      match[1],
				isArrayRef: false,
			})
		} else {
			components = append(components, pathPart{
				value:      match[2],
				isArrayRef: true,
			})
		}
	}
	return components, nil
}

// lookup retrieves a value from the symbol table using a path string
func lookup(path string, scope map[string]any) (any, error) {
	components, err := parsePath(path)
	if err != nil {
		return nil, err
	}

	current := any(scope)
	for _, comp := range components {
		switch v := current.(type) {
		case map[string]any:
			var exists bool
			current, exists = v[comp.value]
			if !exists {
				return nil, fmt.Errorf("key not found: %s", comp.value)
			}

		case []any:
			// Only attempt array indexing if the component was marked as an array reference
			if !comp.isArrayRef {
				return nil, fmt.Errorf("cannot use '%s' as array index: not an array reference", comp.value)
			}

			index, err := strconv.Atoi(comp.value)
			if err != nil {
				return nil, fmt.Errorf("invalid array index: %s", comp.value)
			}
			if index < 0 || index >= len(v) {
				return nil, fmt.Errorf("array index out of bounds: %d", index)
			}
			current = v[index]

		default:
			return nil, fmt.Errorf("cannot navigate through type %T", current)
		}
	}

	return current, nil
}

func handleInnerText(symbols map[string]any, path string) (*html.Node, error) {
	v, err := lookup(path, symbols)
	if err != nil {
		return nil, err
	}
	var str string
	switch u := v.(type) {
	case float64:
		str = fmt.Sprintf("%g", u)
	case string:
		str = u
	default:
		return nil, fmt.Errorf("can not assign '%v' of type %T as inner text", v, v)
	}
	return &html.Node{
		Type: html.TextNode,
		Data: str,
	}, nil
}

// evaluateNode evaluates a single HTML node in the tree.
//
// The returned html nodes will have no parent and no siblings and it
// is thus safe to append them as the child nodes of another HTML node.
func (t *Template) evaluateNode(n *html.Node, symbols map[string]any) ([]*html.Node, error) {
	if n.Type == html.ElementNode && n.Data == "render" {
		return t.evaluateRender(n, symbols)
	}
	if n.Type == html.ElementNode && n.Data == "fragment" {
		return t.evaluateFragment(n, symbols)
	}
	if n.Type == html.ElementNode && n.Data == "children" {
		return t.evaluateChildren(symbols)
	}
	if n.Type == html.ElementNode && n.Data == "for" {
		return t.evaluateFor(n, symbols)
	}
	if n.Type == html.ElementNode && n.Data == "if" {
		return t.evaluateIf(n, symbols)
	}
	return t.evaluateNative(n, symbols)
}

// evaluateChildren evaluates a `children` tag.
// <children></children>
func (t *Template) evaluateChildren(s map[string]any) ([]*html.Node, error) {
	v, err := lookup("children", s)
	if err != nil {
		return nil, err
	}
	switch u := v.(type) {
	case nil:
		return nil, nil
	case []*html.Node:
		return u, nil
	}
	panic("Unexpected type of children")
}

// evaluateFragment evaluates a `fragment` tag.
// <fragment inner-text="item.title"></fragment>
func (t *Template) evaluateFragment(n *html.Node, s map[string]any) ([]*html.Node, error) {
	if len(n.Attr) > 1 {
		panic("Expected fragment to have exactly 0 or 1 attribute after type checking")
	}
	if len(n.Attr) == 1 {
		textNode, err := handleInnerText(s, n.Attr[0].Val)
		return []*html.Node{textNode}, err
	}
	result := []*html.Node{}
	for c := range n.ChildNodes() {
		ns, err := t.evaluateNode(c, s)
		if err != nil {
			return nil, err
		}
		result = append(result, ns...)
	}
	return result, nil
}

// evaluateRender evaluates a `render` tag.
// <render function="list" params="item">
// ...
// </render>
func (t *Template) evaluateRender(n *html.Node, s map[string]any) ([]*html.Node, error) {
	if len(n.Attr) < 1 || len(n.Attr) > 2 {
		panic("Expected render to have exactly 1 or 2 attributes after type checking")
	}
	var function *html.Node
	var valueToBind any
	for _, attr := range n.Attr {
		if attr.Key == "function" {
			c, found := t.functions[attr.Val]
			if !found {
				return nil, fmt.Errorf("no function with name '%s'", attr.Val)
			}
			function = c
		}
		if attr.Key == "params" {
			v, err := lookup(attr.Val, s)
			if err != nil {
				return nil, err
			}
			valueToBind = v
		}
	}
	functionScope := map[string]any{}
	for _, attr := range function.Attr {
		if attr.Key == "params-as" {
			functionScope[attr.Val] = valueToBind
		}
	}
	var children []*html.Node
	for c := range n.ChildNodes() {
		processed, err := t.evaluateNode(c, s)
		if err != nil {
			return nil, err
		}
		children = append(children, processed...)
	}

	// TODO: This data shouldn't live in the function scope
	functionScope["children"] = children

	var results []*html.Node
	for cc := range function.ChildNodes() {
		ns, err := t.evaluateNode(cc, functionScope)
		if err != nil {
			return nil, err
		}
		results = append(results, ns...)
	}

	return results, nil
}

// evaluateIf evaluates an `if` tag:
//
// <if true="item.isActive">
// ...
// </if>
func (t *Template) evaluateIf(n *html.Node, s map[string]any) ([]*html.Node, error) {
	if len(n.Attr) != 1 {
		panic("Expected if to have exactly 1 attribute after type checking")
	}
	v, err := lookup(n.Attr[0].Val, s)
	if err != nil {
		return nil, err
	}
	b, ok := v.(bool)
	if !ok {
		return nil, fmt.Errorf("can not use '%v' of type %T as condition in if", v, v)
	}
	if !b {
		return []*html.Node{}, nil
	}
	var results []*html.Node
	for c := range n.ChildNodes() {
		ns, err := t.evaluateNode(c, s)
		if err != nil {
			return nil, err
		}
		results = append(results, ns...)
	}

	return results, nil
}

// evaluateFor evaluates a `for` tag:
//
// <for each="items" as="item">
// ...
// </for>
func (t *Template) evaluateFor(n *html.Node, s map[string]any) ([]*html.Node, error) {
	if len(n.Attr) < 1 || len(n.Attr) > 2 {
		panic("Expected for to have exactly 1 or 2 attributes after type checking")
	}
	var each string
	var as string
	for _, attr := range n.Attr {
		switch attr.Key {
		case "each":
			each = attr.Val
		case "as":
			as = attr.Val
		}
	}

	v, err := lookup(each, s)
	if err != nil {
		return nil, err
	}

	array, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("can not iterate over '%s' of type %s", stringify(v), typeof(v))
	}

	// Clone the symbol table to allow for mutation.
	if as != "" {
		s = maps.Clone(s)
	}

	var results []*html.Node
	for _, item := range array {
		// Mutation is thread-safe here since we have cloned the symbol table.
		if as != "" {
			s[as] = item
		}
		for c := range n.ChildNodes() {
			ns, err := t.evaluateNode(c, s)
			if err != nil {
				return nil, err
			}
			results = append(results, ns...)
		}
	}

	return results, nil
}

// evaluateNative evaluates a native tag such as a <div>.
func (t *Template) evaluateNative(n *html.Node, s map[string]any) ([]*html.Node, error) {
	result := html.Node{
		Type:     n.Type,
		Data:     n.Data,
		DataAtom: n.DataAtom,
	}

	for _, attr := range n.Attr {
		switch {
		case attr.Key == "inner-text":
			textNode, err := handleInnerText(s, attr.Val)
			if err != nil {
				return nil, err
			}
			result.AppendChild(textNode)
		case strings.HasPrefix(attr.Key, "attr-"):
			v, err := lookup(attr.Val, s)
			if err != nil {
				return nil, err
			}
			str, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("can not use '%s' of type %s as an attribute", stringify(v), typeof(v))
			}
			result.Attr = append(result.Attr, html.Attribute{
				Key: strings.TrimPrefix(attr.Key, "attr-"),
				Val: str,
			})
		default:
			result.Attr = append(result.Attr, attr)
		}
	}

	if result.FirstChild == nil {
		for c := range n.ChildNodes() {
			children, err := t.evaluateNode(c, s)
			if err != nil {
				return nil, err
			}
			for _, child := range children {
				result.AppendChild(child)
			}
		}
	}

	return []*html.Node{&result}, nil
}
