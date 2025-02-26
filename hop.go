package hop

import (
	"encoding/json"
	"fmt"
	"io"
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

// NewProgram takes a template string, parses it and returns
// a program or fails.
func NewProgram() *Program {
	return &Program{modules: map[string]module{}}
}

func (p *Program) AddModule(moduleName string, template string) error {
	parseResult, err := parser.Parse(template)
	if err != nil {
		return err
	}

	functions := map[string]*html.Node{}
	imports := map[string][]string{}

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
		if c.Type == html.ElementNode && c.Data == "import" {
			var module string
			var function string
			for _, attr := range c.Attr {
				if attr.Key == "from" {
					module = attr.Val
				}
				if attr.Key == "function" {
					function = attr.Val
				}
			}
			imports[module] = append(imports[module], function)
		}
	}

	p.modules[moduleName] = module{
		root:          parseResult.Root,
		functions:     functions,
		imports:       imports,
		functionTypes: make(map[string]typechecker.TypeExpr),
		nodePositions: parseResult.NodePositions,
	}

	return nil
}

func (p *Program) Compile() error {
	err := p.typecheckModules()
	if err != nil {
		return err
	}
	return p.resolveImports()
}

// typecheckModules performs typechecking on all modules in topological order
func (p *Program) typecheckModules() error {
	// Create a map of module imports for topological sorting
	moduleImports := make(map[string]map[string][]string)
	for moduleName, mod := range p.modules {
		moduleImports[moduleName] = mod.imports
	}

	// Get modules in topological order (dependencies first)
	sortedModules, err := toposort.TopologicalSortModules(moduleImports)
	if err != nil {
		return fmt.Errorf("sorting modules: %w", err)
	}

	fmt.Printf("%v\n", sortedModules)

	// Process modules in topological order
	for _, moduleName := range sortedModules {
		mod := p.modules[moduleName]

		// Collect imported function types
		importedFunctionTypes := make(map[string]typechecker.TypeExpr)
		for importModuleName, functionNames := range mod.imports {
			importedModule := p.modules[importModuleName]
			for _, functionName := range functionNames {
				importedType, exists := importedModule.functionTypes[functionName]
				if !exists {
					return fmt.Errorf("function %s not found in module %s (imported by %s)",
						functionName, importModuleName, moduleName)
				}
				importedFunctionTypes[functionName] = importedType
			}
		}

		// Typecheck with imported function types
		functionTypes, err := typechecker.Typecheck(mod.root, mod.nodePositions, importedFunctionTypes)
		if err != nil {
			return fmt.Errorf("typechecking module %s: %w", moduleName, err)
		}

		// Store the function types
		mod.functionTypes = functionTypes
		p.modules[moduleName] = mod
	}

	return nil
}

// resolveImports resolves all module imports after typechecking
func (p *Program) resolveImports() error {
	// Create a map of module imports for topological sorting
	moduleImports := make(map[string]map[string][]string)
	for moduleName, mod := range p.modules {
		moduleImports[moduleName] = mod.imports
	}

	// Get modules in topological order (dependencies first)
	sortedModules, err := toposort.TopologicalSortModules(moduleImports)
	if err != nil {
		return fmt.Errorf("sorting modules: %w", err)
	}

	// Process modules in topological order
	for _, moduleName := range sortedModules {
		mod := p.modules[moduleName]

		for importModuleName, functionNames := range mod.imports {
			importedModule := p.modules[importModuleName]

			for _, functionName := range functionNames {
				importedFunction, exists := importedModule.functions[functionName]
				if !exists {
					return fmt.Errorf("function %s not found in module %s (imported by %s)",
						functionName, importModuleName, moduleName)
				}

				// Add the imported function to the current module's functions
				mod.functions[functionName] = importedFunction
			}
		}

		// Update the module in the program
		p.modules[moduleName] = mod
	}

	return nil
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
		nodes, err := module.evaluateNode(c, functionScope)
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
func (m *module) evaluateNode(n *html.Node, symbols map[string]any) ([]*html.Node, error) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "render":
			return m.evaluateRender(n, symbols)
		case "fragment":
			return m.evaluateFragment(n, symbols)
		case "children":
			return m.evaluateChildren(symbols)
		case "for":
			return m.evaluateFor(n, symbols)
		case "if":
			return m.evaluateIf(n, symbols)
		}
	}
	return m.evaluateNative(n, symbols)
}

// evaluateChildren evaluates a `children` tag.
// <children></children>
func (m *module) evaluateChildren(s map[string]any) ([]*html.Node, error) {
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
func (m *module) evaluateFragment(n *html.Node, s map[string]any) ([]*html.Node, error) {
	if len(n.Attr) > 1 {
		panic("Expected fragment to have exactly 0 or 1 attribute after type checking")
	}
	if len(n.Attr) == 1 {
		textNode, err := handleInnerText(s, n.Attr[0].Val)
		return []*html.Node{textNode}, err
	}
	result := []*html.Node{}
	for c := range n.ChildNodes() {
		ns, err := m.evaluateNode(c, s)
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
func (m *module) evaluateRender(n *html.Node, s map[string]any) ([]*html.Node, error) {
	if len(n.Attr) < 1 || len(n.Attr) > 2 {
		panic("Expected render to have exactly 1 or 2 attributes after type checking")
	}
	var function *html.Node
	var valueToBind any
	for _, attr := range n.Attr {
		if attr.Key == "function" {
			c, found := m.functions[attr.Val]
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
		processed, err := m.evaluateNode(c, s)
		if err != nil {
			return nil, err
		}
		children = append(children, processed...)
	}

	// TODO: This data shouldn't live in the function scope
	functionScope["children"] = children

	var results []*html.Node
	for cc := range function.ChildNodes() {
		ns, err := m.evaluateNode(cc, functionScope)
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
func (m *module) evaluateIf(n *html.Node, s map[string]any) ([]*html.Node, error) {
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
		ns, err := m.evaluateNode(c, s)
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
func (m *module) evaluateFor(n *html.Node, s map[string]any) ([]*html.Node, error) {
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
			ns, err := m.evaluateNode(c, s)
			if err != nil {
				return nil, err
			}
			results = append(results, ns...)
		}
	}

	return results, nil
}

// evaluateNative evaluates a native tag such as a <div>.
func (m *module) evaluateNative(n *html.Node, s map[string]any) ([]*html.Node, error) {
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
			children, err := m.evaluateNode(c, s)
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
