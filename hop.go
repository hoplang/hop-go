package hop

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"reflect"
	"strconv"
	"strings"

	"github.com/hoplang/hop-go/internal/toposort"
	"github.com/hoplang/hop-go/parser"
	"github.com/hoplang/hop-go/typechecker"
	"golang.org/x/net/html"
)

type function struct {
	children []*html.Node
	name     string
}

type module struct {
	root          *html.Node
	functions     map[string]*html.Node
	imports       map[string][]string
	functionTypes map[string]typechecker.TypeExpr
	nodePositions map[*html.Node]parser.NodePosition
}

type Program struct {
	modules map[string]module
}

type Compiler struct {
	modules map[string]string
}

func NewCompiler() *Compiler {
	return &Compiler{
		modules: map[string]string{},
	}
}

func (c *Compiler) AddFS(fsys fs.FS) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".hop") {
			return nil
		}
		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		moduleName := strings.TrimSuffix(path, ".hop")
		c.AddModule(moduleName, string(content))
		return nil
	})
}

func (c *Compiler) AddModule(moduleName string, template string) {
	c.modules[moduleName] = template
}

func (c *Compiler) Compile() (*Program, error) {
	p := &Program{
		modules: map[string]module{},
	}

	// Step 1: Parse all modules and collect dependencies
	moduleImports := make(map[string]map[string][]string)

	for moduleName, templateSrc := range c.modules {
		parseResult, err := parser.Parse(templateSrc)
		if err != nil {
			return nil, fmt.Errorf("parsing module %s: %w", moduleName, err)
		}

		mod := module{
			root:          parseResult.Root,
			functions:     map[string]*html.Node{},
			imports:       map[string][]string{},
			functionTypes: map[string]typechecker.TypeExpr{},
			nodePositions: parseResult.NodePositions,
		}

		for c := range parseResult.Root.ChildNodes() {
			if c.Type != html.ElementNode {
				continue
			}

			switch c.Data {
			case "function":
				for _, attr := range c.Attr {
					if attr.Key == "name" {
						mod.functions[attr.Val] = c
						break
					}
				}
			case "import":
				var module, function string
				for _, attr := range c.Attr {
					if attr.Key == "from" {
						module = attr.Val
					} else if attr.Key == "function" {
						function = attr.Val
					}
				}
				mod.imports[module] = append(mod.imports[module], function)
			}
		}

		p.modules[moduleName] = mod
		moduleImports[moduleName] = mod.imports
	}

	// Step 2: Process modules in dependency order
	sortedModules, err := toposort.TopologicalSortModules(moduleImports)
	if err != nil {
		return nil, fmt.Errorf("sorting modules: %w", err)
	}

	for _, moduleName := range sortedModules {
		mod := p.modules[moduleName]
		importedFunctionTypes := make(map[string]typechecker.TypeExpr)

		// Process imports
		for importModuleName, functionNames := range mod.imports {
			importedModule := p.modules[importModuleName]
			for _, functionName := range functionNames {
				// Get function type and implementation
				if importedType, ok := importedModule.functionTypes[functionName]; ok {
					importedFunctionTypes[functionName] = importedType
				} else {
					return nil, fmt.Errorf("function %s not found in module %s",
						functionName, importModuleName)
				}
			}
		}

		// Typecheck
		functionTypes, err := typechecker.Typecheck(mod.root, mod.nodePositions, importedFunctionTypes)
		if err != nil {
			return nil, fmt.Errorf("typechecking module %s: %w", moduleName, err)
		}

		mod.functionTypes = functionTypes
		p.modules[moduleName] = mod
	}

	return p, nil
}

// ExecuteFunction executes a specific function from the template with the given parameters
func (p *Program) ExecuteFunction(w io.Writer, moduleName string, functionName string, data any) error {
	module, exists := p.modules[moduleName]
	if !exists {
		return fmt.Errorf("no module with name %s", moduleName)
	}
	function, exists := module.functions[functionName]
	if !exists {
		return fmt.Errorf("no function with name %s in module %s", functionName, moduleName)
	}
	functionScope := map[string]any{}
	for _, attr := range function.Attr {
		if attr.Key == "params-as" {
			functionScope[attr.Val] = data
		}
	}
	for c := range function.ChildNodes() {
		nodes, err := p.evaluateNode(moduleName, c, functionScope)
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
func (p *Program) evaluateNode(currentModule string, n *html.Node, symbols map[string]any) ([]*html.Node, error) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "render":
			return p.evaluateRender(currentModule, n, symbols)
		case "fragment":
			return p.evaluateFragment(currentModule, n, symbols)
		case "children":
			return p.evaluateChildren(symbols)
		case "for":
			return p.evaluateFor(currentModule, n, symbols)
		case "if":
			return p.evaluateIf(currentModule, n, symbols)
		}
	}
	return p.evaluateNative(currentModule, n, symbols)
}

// evaluateChildren evaluates a `children` tag.
// <children></children>
func (p *Program) evaluateChildren(s map[string]any) ([]*html.Node, error) {
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
func (p *Program) evaluateFragment(currentModule string, n *html.Node, s map[string]any) ([]*html.Node, error) {
	if len(n.Attr) > 1 {
		panic("Expected fragment to have exactly 0 or 1 attribute after type checking")
	}
	if len(n.Attr) == 1 {
		textNode, err := handleInnerText(s, n.Attr[0].Val)
		return []*html.Node{textNode}, err
	}
	result := []*html.Node{}
	for c := range n.ChildNodes() {
		ns, err := p.evaluateNode(currentModule, c, s)
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
func (p *Program) evaluateRender(currentModule string, n *html.Node, s map[string]any) ([]*html.Node, error) {
	if len(n.Attr) < 1 || len(n.Attr) > 2 {
		panic("Expected render to have exactly 1 or 2 attributes after type checking")
	}

	var functionName string
	var valueToBind any

	for _, attr := range n.Attr {
		if attr.Key == "function" {
			functionName = attr.Val
		}
		if attr.Key == "params" {
			v, err := lookup(attr.Val, s)
			if err != nil {
				return nil, err
			}
			valueToBind = v
		}
	}

	// Determine which module contains the function
	targetModule := currentModule
	targetFunction := functionName

	// Check if the function is imported from another module
	mod := p.modules[currentModule]
	for importedModule, functions := range mod.imports {
		for _, fn := range functions {
			if fn == functionName {
				targetModule = importedModule
				break
			}
		}
	}

	// Get the function from the correct module
	function, found := p.modules[targetModule].functions[targetFunction]
	if !found {
		return nil, fmt.Errorf("no function with name '%s' in module '%s'", targetFunction, targetModule)
	}

	functionScope := map[string]any{}
	for _, attr := range function.Attr {
		if attr.Key == "params-as" {
			functionScope[attr.Val] = valueToBind
		}
	}

	var children []*html.Node
	for c := range n.ChildNodes() {
		processed, err := p.evaluateNode(currentModule, c, s)
		if err != nil {
			return nil, err
		}
		children = append(children, processed...)
	}

	// Add children to the function scope
	functionScope["children"] = children

	var results []*html.Node
	for cc := range function.ChildNodes() {
		ns, err := p.evaluateNode(targetModule, cc, functionScope)
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
func (p *Program) evaluateIf(currentModule string, n *html.Node, s map[string]any) ([]*html.Node, error) {
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
		ns, err := p.evaluateNode(currentModule, c, s)
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
func (p *Program) evaluateFor(currentModule string, n *html.Node, s map[string]any) ([]*html.Node, error) {
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
			ns, err := p.evaluateNode(currentModule, c, s)
			if err != nil {
				return nil, err
			}
			results = append(results, ns...)
		}
	}

	return results, nil
}

// evaluateNative evaluates a native tag such as a <div>.
func (p *Program) evaluateNative(currentModule string, n *html.Node, s map[string]any) ([]*html.Node, error) {
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
			children, err := p.evaluateNode(currentModule, c, s)
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
