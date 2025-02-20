package typechecker

import (
	"fmt"
	"maps"
	"strings"

	"github.com/hoplang/hop-go/parser"
	"github.com/hoplang/hop-go/typechecker/internal/toposort"
	"golang.org/x/net/html"
)

// TypeExpr represents a type expression in our system
type TypeExpr interface {
	String() string
}

// TypeVar represents a type variable (unknown type)
type TypeVar struct {
	Name string
	Link *TypeExpr // For unification
}

func (tv *TypeVar) String() string {
	if tv.Link != nil {
		return (*tv.Link).String()
	}
	return "?" + tv.Name
}

// PrimitiveType represents basic types like string, number, boolean
type PrimitiveType string

func (pt PrimitiveType) String() string {
	return string(pt)
}

// ArrayType represents an array of some type
type ArrayType struct {
	ElementType TypeExpr
}

func (at *ArrayType) String() string {
	return fmt.Sprintf("[]%s", at.ElementType)
}

// ObjectType represents a type with fields
type ObjectType struct {
	Fields map[string]TypeExpr
}

func (ot *ObjectType) String() string {
	fields := make([]string, 0, len(ot.Fields))
	for name, typ := range ot.Fields {
		fields = append(fields, fmt.Sprintf("%s: %s", name, typ))
	}
	return fmt.Sprintf("{%s}", strings.Join(fields, ", "))
}

// UnionType represents a type that could be one of several types
type UnionType struct {
	Types []TypeExpr
}

func (ut *UnionType) String() string {
	types := make([]string, len(ut.Types))
	for i, t := range ut.Types {
		types[i] = t.String()
	}
	return strings.Join(types, " | ")
}

// TypeError represents a type mismatch in template usage
type TypeError struct {
	Context string
	Path    []string
}

func (e *TypeError) Error() string {
	if len(e.Path) > 0 {
		return fmt.Sprintf("type error in %s: %s", strings.Join(e.Path, "."), e.Context)
	}
	return fmt.Sprintf("type error: %s", e.Context)
}

// TypeChecker handles type checking, inference and unification
type TypeChecker struct {
	nextVar        int
	functionParams map[string]TypeExpr
}

func NewTypeChecker() *TypeChecker {
	return &TypeChecker{
		nextVar:        0,
		functionParams: make(map[string]TypeExpr),
	}
}

func (tc *TypeChecker) NewVar() *TypeVar {
	tc.nextVar++
	return &TypeVar{Name: fmt.Sprintf("t%d", tc.nextVar)}
}

// unify attempts to unify two types
func (tc *TypeChecker) unify(t1, t2 TypeExpr) error {
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

// typecheckLookup type checks a lookup inside a scope.
//
// E.g. given a scope {foo: ?t1} and a path "foo.bar" it will unify
// {foo: ?t1} and {foo: {bar: ?t2}} yielding {foo: {bar: ?t2}} as the
// new state for the scope. It then returns ?t2.
func (tc *TypeChecker) typecheckLookup(path string, scope map[string]TypeExpr) (TypeExpr, error) {
	parts, err := parser.ParsePath(path)
	if err != nil {
		return nil, err
	}

	if parts[0].IsArrayRef {
		return nil, &TypeError{Context: fmt.Sprintf("unexpected array-index")}
	}

	currentType, exists := scope[parts[0].Value]
	if !exists {
		return nil, &TypeError{Context: fmt.Sprintf("undefined variable '%s'", parts[0].Value)}
	}

	for _, comp := range parts[1:] {
		if comp.IsArrayRef {
			arrayType := &ArrayType{ElementType: tc.NewVar()}
			if err := tc.unify(currentType, arrayType); err != nil {
				return nil, &TypeError{Context: fmt.Sprintf("cannot index non-array value: %s", err)}
			}
			currentType = arrayType.ElementType
		} else {
			fieldType := tc.NewVar()
			objType := &ObjectType{Fields: map[string]TypeExpr{comp.Value: fieldType}}
			if err := tc.unify(currentType, objType); err != nil {
				return nil, &TypeError{Context: fmt.Sprintf("cannot access field '%s': %s", comp.Value, err)}
			}
			currentType = fieldType
		}
	}

	return currentType, nil
}

// inferTypes infers the types of all functions of a template.
func InferTypes(functions map[string]*html.Node) (map[string]TypeExpr, error) {
	sortedFunctions, err := toposort.TopologicalSort(functions)
	if err != nil {
		return nil, err
	}
	tc := NewTypeChecker()
	for _, name := range sortedFunctions {
		function := functions[name]
		s := map[string]TypeExpr{}
		if paramsAs, found := getAttribute(function, "params-as"); found {
			tc.functionParams[name] = tc.NewVar()
			s[paramsAs] = tc.functionParams[name]
		} else {
			// Void is used for functions without a params-as attribute
			tc.functionParams[name] = PrimitiveType("void")
		}
		if err := tc.typecheckNode(function, s); err != nil {
			return nil, err
		}
	}
	return tc.functionParams, nil
}

func (tc *TypeChecker) typecheckNode(n *html.Node, s map[string]TypeExpr) error {
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

func (tc *TypeChecker) typecheckNative(n *html.Node, s map[string]TypeExpr) error {
	for _, attr := range n.Attr {
		if attr.Key == "inner-text" || strings.HasPrefix(attr.Key, "attr-") {
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
				return &TypeError{Context: fmt.Sprintf("invalid type for %s binding: %s", attr.Key, err)}
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

func (tc *TypeChecker) typecheckFragment(n *html.Node, s map[string]TypeExpr) error {
	for _, attr := range n.Attr {
		switch attr.Key {
		case "inner-text":
		default:
			return &TypeError{Context: fmt.Sprintf("unrecognized attribute '%s' in %s", attr.Key, n.Data)}
		}
	}
	for c := range n.ChildNodes() {
		if err := tc.typecheckNode(c, s); err != nil {
			return err
		}
	}
	return nil
}

func (tc *TypeChecker) typecheckFor(n *html.Node, s map[string]TypeExpr) error {
	var each, as string
	for _, attr := range n.Attr {
		switch attr.Key {
		case "each":
			each = attr.Val
		case "as":
			as = attr.Val
		default:
			return &TypeError{Context: fmt.Sprintf("unrecognized attribute '%s' in %s", attr.Key, n.Data)}
		}
	}

	if each == "" {
		return &TypeError{Context: "for loop missing 'each' attribute"}
	}

	iterType, err := tc.typecheckLookup(each, s)
	if err != nil {
		return err
	}

	elemType := tc.NewVar()

	if err := tc.unify(iterType, &ArrayType{ElementType: elemType}); err != nil {
		return &TypeError{Context: fmt.Sprintf("cannot iterate over non-array value: %s", err)}
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

func (tc *TypeChecker) typecheckIf(n *html.Node, s map[string]TypeExpr) error {
	var cond string
	for _, attr := range n.Attr {
		switch attr.Key {
		case "true":
			cond = attr.Val
		default:
			return &TypeError{Context: fmt.Sprintf("unrecognized attribute '%s' in %s", attr.Key, n.Data)}
		}
	}

	if cond == "" {
		return &TypeError{Context: fmt.Sprintf("empty condition in if")}
	}

	condType, err := tc.typecheckLookup(cond, s)
	if err != nil {
		return err
	}

	if err := tc.unify(condType, PrimitiveType("boolean")); err != nil {
		return &TypeError{Context: fmt.Sprintf("condition must be boolean: %s", err)}
	}

	for c := range n.ChildNodes() {
		if err := tc.typecheckNode(c, s); err != nil {
			return err
		}
	}
	return nil
}

func (tc *TypeChecker) typecheckRender(n *html.Node, s map[string]TypeExpr) error {
	functionName, ok := getAttribute(n, "function")
	if !ok {
		return &TypeError{Context: "render is missing attribute 'function'"}
	}

	params, found := getAttribute(n, "params")
	if found {
		paramsType, err := tc.typecheckLookup(params, s)
		if err != nil {
			return err
		}

		if err := tc.unify(paramsType, tc.functionParams[functionName]); err != nil {
			return &TypeError{Context: fmt.Sprintf("invalid parameter type for function '%s': %s", functionName, err)}
		}
	} else {
		if tc.functionParams[functionName] != PrimitiveType("void") {
			return &TypeError{Context: fmt.Sprintf("missing attribute params in render call for %s", functionName)}
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
