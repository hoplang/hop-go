package typechecker

import (
	"fmt"
	"strings"

	"github.com/hoplang/hop-go/parser"
	"golang.org/x/net/html"
)

// TypeError represents a type mismatch in template usage
type TypeError struct {
	Start   parser.Position
	End     parser.Position
	Context string
	Path    []string
}

func (e *TypeError) Error() string {
	if len(e.Path) > 0 {
		return fmt.Sprintf("%s-%s: type error in %s: %s",
			e.Start, e.End, strings.Join(e.Path, "."), e.Context)
	}
	return fmt.Sprintf("%s-%s: type error: %s", e.Start, e.End, e.Context)
}

// Helper to create type errors with position information
func (tc *typeChecker) newError(node *html.Node, format string, args ...interface{}) *TypeError {
	start := parser.Position{Line: 0, Column: 0}
	end := parser.Position{Line: 0, Column: 0}
	if nodePos, exists := tc.nodePositions[node]; exists {
		start = nodePos.Start
		end = nodePos.End
	}
	return &TypeError{
		Start:   start,
		End:     end,
		Context: fmt.Sprintf(format, args...),
	}
}

func (tc *typeChecker) newErrorForAttr(node *html.Node, attrName string, format string, args ...interface{}) *TypeError {
	start := parser.Position{Line: 0, Column: 0}
	end := parser.Position{Line: 0, Column: 0}

	if nodePos, exists := tc.nodePositions[node]; exists {
		if attrPos, exists := nodePos.Attributes[attrName]; exists {
			if attrPos.ValueStart.Line > 0 {
				start = attrPos.ValueStart
				end = attrPos.ValueEnd
			} else {
				start = attrPos.NameStart
				end = attrPos.NameEnd
			}
		} else {
			start = nodePos.Start
			end = nodePos.End
		}
	}

	return &TypeError{
		Start:   start,
		End:     end,
		Context: fmt.Sprintf(format, args...),
	}
}
