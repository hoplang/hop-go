package parser

import "regexp"

// PathPart represents a part of a path with information
// about whether it's an array index
type PathPart struct {
	Value      string
	IsArrayRef bool
}

// Match either:
// 1. a segment between dots or at start/end of string
// 2. anything in square brackets
var pathPartRegexp = regexp.MustCompile(`([^.\[\]]+)|\[([^\]]+)\]`)

// ParsePath splits a path string into parts.
//
// Examples:
//
//	"foo.bar" => [{foo false} {bar false}]
//	"foo.bar[0].baz" => [{foo false} {bar true} {baz false}]
//	"foo[0][1][2]" => [{foo true} {1 true} {2 true}]
func ParsePath(path string) ([]PathPart, error) {
	matches := pathPartRegexp.FindAllStringSubmatch(path, -1)
	components := []PathPart{}
	for _, match := range matches {
		if match[1] != "" {
			components = append(components, PathPart{
				Value:      match[1],
				IsArrayRef: false,
			})
		} else {
			components = append(components, PathPart{
				Value:      match[2],
				IsArrayRef: true,
			})
		}
	}
	return components, nil
}
