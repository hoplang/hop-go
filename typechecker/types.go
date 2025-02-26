package typechecker

import (
	"fmt"
	"strings"
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
