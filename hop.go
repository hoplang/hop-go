package hop

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"reflect"
	"strconv"
	"strings"

	"github.com/hoplang/hop-go/parser"
	"github.com/hoplang/hop-go/typechecker"
	"golang.org/x/net/html"
)

type Program struct {
	root      *html.Node
	functions map[string]*html.Node
}

type Function struct {
	children []*html.Node
	name     string
}

// NewProgram takes a template string, parses it and returns
// a program or fails.
func NewProgram(template string) (*Program, error) {
	parseResult, err := parser.Parse(template)
	if err != nil {
		return nil, err
	}

	// Use typechecker to handle function collection and type checking
	_, err = typechecker.Typecheck(parseResult.Root, parseResult.NodePositions)
	if err != nil {
		return nil, err
	}

	functions := map[string]*html.Node{}
	for c := range parseResult.Root.ChildNodes() {
		if c.Type == html.ElementNode && c.Data == "function" {
			var name string
			for _, attr := range c.Attr {
				if attr.Key == "name" {
					name = attr.Val
				}
			}
			functions[name] = c
		}
	}

	return &Program{root: parseResult.Root, functions: functions}, nil
}

// ExecuteFunction executes a specific function from the template with the given parameters
func (t *Program) ExecuteFunction(w io.Writer, functionName string, data any) error {
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

func getFieldByJSONTag(v reflect.Value, tagName string) (reflect.Value, error) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		// Split the json tag to handle cases like `json:"name,omitempty"`
		tagParts := strings.Split(jsonTag, ",")
		if tagParts[0] == tagName {
			return v.Field(i), nil
		}
	}
	return reflect.Value{}, fmt.Errorf("json tag %s not found", tagName)
}

// lookup retrieves a value from the symbol table using a path string
func lookup(path string, scope map[string]any) (any, error) {
	components, err := parser.ParsePath(path)
	if err != nil {
		return nil, err
	}

	current := any(scope)
	for _, comp := range components {
		switch v := current.(type) {
		case map[string]any:
			var exists bool
			current, exists = v[comp.Value]
			if !exists {
				return nil, fmt.Errorf("key not found: %s", comp.Value)
			}

		case []any:
			// Only attempt array indexing if the component was marked as an array reference
			if !comp.IsArrayRef {
				return nil, fmt.Errorf("cannot use '%s' as array index: not an array reference", comp.Value)
			}

			index, err := strconv.Atoi(comp.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid array index: %s", comp.Value)
			}
			if index < 0 || index >= len(v) {
				return nil, fmt.Errorf("array index out of bounds: %d", index)
			}
			current = v[index]

		default:
			val := reflect.ValueOf(current)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}

			if val.Kind() == reflect.Struct {
				field, err := getFieldByJSONTag(val, comp.Value)
				if err != nil {
					return nil, err
				}
				if !field.CanInterface() {
					return nil, fmt.Errorf("field with json tag %s is not exported", comp.Value)
				}
				current = field.Interface()
			} else {
				return nil, fmt.Errorf("cannot navigate through type %T", current)
			}
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
	case int:
		str = fmt.Sprintf("%d", u)
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
func (t *Program) evaluateNode(n *html.Node, symbols map[string]any) ([]*html.Node, error) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "render":
			return t.evaluateRender(n, symbols)
		case "fragment":
			return t.evaluateFragment(n, symbols)
		case "children":
			return t.evaluateChildren(symbols)
		case "for":
			return t.evaluateFor(n, symbols)
		case "if":
			return t.evaluateIf(n, symbols)
		}
	}
	return t.evaluateNative(n, symbols)
}

// evaluateChildren evaluates a `children` tag.
// <children></children>
func (t *Program) evaluateChildren(s map[string]any) ([]*html.Node, error) {
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
func (t *Program) evaluateFragment(n *html.Node, s map[string]any) ([]*html.Node, error) {
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
func (t *Program) evaluateRender(n *html.Node, s map[string]any) ([]*html.Node, error) {
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
func (t *Program) evaluateIf(n *html.Node, s map[string]any) ([]*html.Node, error) {
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
func (t *Program) evaluateFor(n *html.Node, s map[string]any) ([]*html.Node, error) {
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

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, fmt.Errorf("can not iterate over '%s' of type %s %v", stringify(v), typeof(v), reflect.TypeOf(v))
	}

	// Clone the symbol table to allow for mutation.
	if as != "" {
		s = maps.Clone(s)
	}

	var results []*html.Node
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()
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
func (t *Program) evaluateNative(n *html.Node, s map[string]any) ([]*html.Node, error) {
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
			var str string
			switch u := v.(type) {
			case float64:
				str = fmt.Sprintf("%g", u)
			case int:
				str = fmt.Sprintf("%d", u)
			case string:
				str = u
			default:
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
