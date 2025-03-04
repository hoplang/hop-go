package typechecker

import (
	"fmt"
	"maps"
	"strings"

	"github.com/hoplang/hop-go/internal/toposort"
	"github.com/hoplang/hop-go/parser"
	"golang.org/x/net/html"
)

type typeChecker struct {
	nextVar        int
	functionParams map[string]TypeExpr
	nodePositions  map[*html.Node]parser.NodePosition
}

func newTypeChecker(positions map[*html.Node]parser.NodePosition) *typeChecker {
	return &typeChecker{
		nextVar:        0,
		functionParams: make(map[string]TypeExpr),
		nodePositions:  positions,
	}
}

func (tc *typeChecker) newVar() *TypeVar {
	tc.nextVar++
	return &TypeVar{Name: fmt.Sprintf("t%d", tc.nextVar)}
}

// unify attempts to unify two types
func (tc *typeChecker) unify(t1, t2 TypeExpr) error {
	if t1 == t2 {
		return nil
	}
	// Dereference type variables
	if tv1, ok := t1.(*TypeVar); ok && tv1.Link != nil {
		return tc.unify(*tv1.Link, t2)
	}
	if tv2, ok := t2.(*TypeVar); ok && tv2.Link != nil {
		return tc.unify(t1, *tv2.Link)
	}

	// Handle type variables
	if tv1, ok := t1.(*TypeVar); ok {
		tv1.Link = &t2
		return nil
	}
	if _, ok := t2.(*TypeVar); ok {
		return tc.unify(t2, t1)
	}

	// Handle concrete types
	switch t1 := t1.(type) {
	case PrimitiveType:
		if t2, ok := t2.(PrimitiveType); ok && t1 == t2 {
			return nil
		}
	case *ArrayType:
		if t2, ok := t2.(*ArrayType); ok {
			return tc.unify(t1.ElementType, t2.ElementType)
		}
	case *ObjectType:
		if t2, ok := t2.(*ObjectType); ok {
			mergedFields := maps.Clone(t1.Fields)
			for name, typ2 := range t2.Fields {
				if typ1, exists := mergedFields[name]; exists {
					if err := tc.unify(typ1, typ2); err != nil {
						return fmt.Errorf("field %s: %w", name, err)
					}
				} else {
					mergedFields[name] = typ2
				}
			}
			t1.Fields = mergedFields
			return nil
		}
	case *UnionType:
		if t2, ok := t2.(*UnionType); ok {
			for _, type1 := range t1.Types {
				for _, type2 := range t2.Types {
					if err := tc.unify(type1, type2); err == nil {
						return nil
					}
				}
			}
		} else {
			for _, type1 := range t1.Types {
				if err := tc.unify(type1, t2); err == nil {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("cannot unify %v with %v", t1, t2)
}

func constructDependencyGraph(root *html.Node) map[string]map[string]bool {
	deps := map[string]map[string]bool{}
	var findRenders func(n *html.Node, source string)
	findRenders = func(n *html.Node, source string) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "render":
				for _, attr := range n.Attr {
					if attr.Key == "function" {
						deps[source][attr.Val] = true
					}
				}
			}
		}
		for c := range n.ChildNodes() {
			findRenders(c, source)
		}
	}
	for c := range root.ChildNodes() {
		if c.Type == html.ElementNode && c.Data == "import" {
			var name string
			for _, attr := range c.Attr {
				if attr.Key == "function" {
					name = attr.Val
				}
			}
			deps[name] = map[string]bool{}
		}
		if c.Type == html.ElementNode && c.Data == "function" {
			var name string
			for _, attr := range c.Attr {
				if attr.Key == "name" {
					name = attr.Val
				}
			}
			deps[name] = map[string]bool{}
			for cc := range c.ChildNodes() {
				findRenders(cc, name)
			}
		}
	}
	return deps
}

// Typecheck infers the types of all functions of a template.
func Typecheck(root *html.Node, positions map[*html.Node]parser.NodePosition, importedFunctions map[string]TypeExpr) (map[string]TypeExpr, error) {
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

	dependencyGraph := constructDependencyGraph(root)

	sortedFunctions, err := toposort.TopologicalSort(dependencyGraph, "function")
	if err != nil {
		return nil, fmt.Errorf("type error: %w", err)
	}

	// Type check functions
	tc := newTypeChecker(positions)

	// Add imported functions to the function params
	for name, typeExpr := range importedFunctions {
		tc.functionParams[name] = typeExpr
	}

	for _, name := range sortedFunctions {
		function, ok := functions[name]
		if !ok {
			continue
		}
		s := map[string]TypeExpr{}
		if paramsAs, found := getAttribute(function, "params-as"); found {
			tc.functionParams[name] = tc.newVar()
			s[paramsAs] = tc.functionParams[name]
		} else {
			tc.functionParams[name] = PrimitiveType("void")
		}
		if err := tc.typecheckNode(function, s); err != nil {
			return nil, err
		}
	}
	return tc.functionParams, nil
}

func (tc *typeChecker) typecheckNode(n *html.Node, s map[string]TypeExpr) error {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "fragment":
			return tc.typecheckFragment(n, s)
		case "for":
			return tc.typecheckFor(n, s)
		case "if":
			return tc.typecheckIf(n, s)
		case "render":
			return tc.typecheckRender(n, s)
		default:
			return tc.typecheckNative(n, s)
		}
	}
	return nil
}

func (tc *typeChecker) typecheckLookup(path string, scope map[string]TypeExpr) (TypeExpr, error) {
	parts, err := parser.ParsePath(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	if parts[0].IsArrayRef {
		return nil, fmt.Errorf("unexpected array-index")
	}

	currentType, exists := scope[parts[0].Value]
	if !exists {
		return nil, fmt.Errorf("undefined variable '%s'", parts[0].Value)
	}

	for _, comp := range parts[1:] {
		if comp.IsArrayRef {
			arrayType := &ArrayType{ElementType: tc.newVar()}
			if err := tc.unify(currentType, arrayType); err != nil {
				return nil, fmt.Errorf("cannot index non-array value: %s", err)
			}
			currentType = arrayType.ElementType
		} else {
			fieldType := tc.newVar()
			objType := &ObjectType{Fields: map[string]TypeExpr{comp.Value: fieldType}}
			if err := tc.unify(currentType, objType); err != nil {
				return nil, fmt.Errorf("cannot access field '%s': %s", comp.Value, err)
			}
			currentType = fieldType
		}
	}

	return currentType, nil
}

func (tc *typeChecker) typecheckNative(n *html.Node, s map[string]TypeExpr) error {
	for _, attr := range n.Attr {
		if attr.Key == "inner-text" || strings.HasPrefix(attr.Key, "attr-") {
			exprType, err := tc.typecheckLookup(attr.Val, s)
			if err != nil {
				return tc.newErrorForAttr(n, attr.Key, "%s", err)
			}

			stringOrNumber := &UnionType{
				Types: []TypeExpr{
					PrimitiveType("string"),
					PrimitiveType("number"),
				},
			}

			if err := tc.unify(exprType, stringOrNumber); err != nil {
				return tc.newErrorForAttr(n, attr.Key, "invalid type for %s binding: %s", attr.Key, err)
			}
		}
	}
	for c := range n.ChildNodes() {
		if err := tc.typecheckNode(c, s); err != nil {
			return err
		}
	}
	return nil
}

func (tc *typeChecker) typecheckFragment(n *html.Node, s map[string]TypeExpr) error {
	for _, attr := range n.Attr {
		switch attr.Key {
		case "inner-text":
			exprType, err := tc.typecheckLookup(attr.Val, s)
			if err != nil {
				return err
			}
			stringOrNumber := &UnionType{
				Types: []TypeExpr{
					PrimitiveType("string"),
					PrimitiveType("number"),
				},
			}
			if err := tc.unify(exprType, stringOrNumber); err != nil {
				return tc.newError(n, "invalid type for inner-text: %s", err)
			}
		default:
			return tc.newError(n, "unrecognized attribute '%s' in %s", attr.Key, n.Data)
		}
	}
	for c := range n.ChildNodes() {
		if err := tc.typecheckNode(c, s); err != nil {
			return err
		}
	}
	return nil
}

func (tc *typeChecker) typecheckFor(n *html.Node, s map[string]TypeExpr) error {
	var each, as string
	for _, attr := range n.Attr {
		switch attr.Key {
		case "each":
			each = attr.Val
		case "as":
			as = attr.Val
		default:
			return tc.newError(n, "unrecognized attribute '%s' in %s", attr.Key, n.Data)
		}
	}

	if each == "" {
		return tc.newError(n, "for loop missing 'each' attribute")
	}

	iterType, err := tc.typecheckLookup(each, s)
	if err != nil {
		return tc.newErrorForAttr(n, "each", "%s", err)
	}

	elemType := tc.newVar()

	if err := tc.unify(iterType, &ArrayType{ElementType: elemType}); err != nil {
		return tc.newErrorForAttr(n, "each", "cannot iterate over non-array value: %s", err)
	}

	if as != "" {
		s = maps.Clone(s)
		s[as] = elemType
	}
	for c := range n.ChildNodes() {
		if err := tc.typecheckNode(c, s); err != nil {
			return err
		}
	}
	return nil
}

func (tc *typeChecker) typecheckIf(n *html.Node, s map[string]TypeExpr) error {
	var cond string
	for _, attr := range n.Attr {
		switch attr.Key {
		case "true":
			cond = attr.Val
		default:
			return tc.newError(n, "unrecognized attribute '%s' in %s", attr.Key, n.Data)
		}
	}

	if cond == "" {
		return tc.newErrorForAttr(n, "true", "empty condition in if")
	}

	condType, err := tc.typecheckLookup(cond, s)
	if err != nil {
		return tc.newErrorForAttr(n, "true", "%s", err)
	}

	if err := tc.unify(condType, PrimitiveType("boolean")); err != nil {
		return tc.newErrorForAttr(n, "true", "condition must be boolean: %s", err)
	}

	for c := range n.ChildNodes() {
		if err := tc.typecheckNode(c, s); err != nil {
			return err
		}
	}
	return nil
}

func (tc *typeChecker) typecheckRender(n *html.Node, s map[string]TypeExpr) error {
	functionName, ok := getAttribute(n, "function")
	if !ok {
		return tc.newError(n, "render is missing attribute 'function'")
	}

	params, found := getAttribute(n, "params")
	if found {
		paramsType, err := tc.typecheckLookup(params, s)
		if err != nil {
			return tc.newErrorForAttr(n, "params", "%s", err)
		}

		if err := tc.unify(paramsType, tc.functionParams[functionName]); err != nil {
			return tc.newError(n, "invalid parameter type for function '%s': %s", functionName, err)
		}
	} else {
		if tc.functionParams[functionName] != PrimitiveType("void") {
			return tc.newError(n, "missing attribute params in render call for %s", functionName)
		}
	}

	for c := range n.ChildNodes() {
		if err := tc.typecheckNode(c, s); err != nil {
			return err
		}
	}
	return nil
}

func getAttribute(node *html.Node, key string) (string, bool) {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val, true
		}
	}
	return "", false
}
