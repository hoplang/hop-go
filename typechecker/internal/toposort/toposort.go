package toposort

import (
	"fmt"
	"slices"

	"golang.org/x/net/html"
)

func getAttribute(node *html.Node, key string) (string, bool) {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val, true
		}
	}
	return "", false
}

// FunctionDependency represents a dependency between template functions
type FunctionDependency struct {
	Name         string
	Dependencies map[string]bool
}

// findFunctionDependencies collects all function dependencies from a node
func findFunctionDependencies(n *html.Node, deps map[string]bool) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "render":
			if functionName, ok := getAttribute(n, "function"); ok {
				deps[functionName] = true
			}
		}
	}

	for c := range n.ChildNodes() {
		findFunctionDependencies(c, deps)
	}
}

// topologicalSort performs a topological sort of function dependencies
func TopologicalSort(functions map[string]*html.Node) ([]string, error) {
	// Build dependency graph
	graph := make(map[string]*FunctionDependency)
	for name := range functions {
		// Initialize all functions in the graph first
		graph[name] = &FunctionDependency{
			Name:         name,
			Dependencies: make(map[string]bool),
		}
	}

	// Now collect dependencies
	for name, function := range functions {
		deps := make(map[string]bool)
		findFunctionDependencies(function, deps)
		graph[name].Dependencies = deps
	}

	// Kahn's algorithm for topological sorting
	var result []string
	inDegree := make(map[string]int)

	// Calculate in-degrees
	for _, dep := range graph {
		for d := range dep.Dependencies {
			if _, exists := graph[d]; !exists {
				return nil, fmt.Errorf("type error: function '%s' depends on undefined function '%s'", dep.Name, d)
			}
			inDegree[d]++
		}
	}

	// Find all nodes with in-degree 0
	var queue []string
	for name := range graph {
		if inDegree[name] == 0 {
			queue = append(queue, name)
		}
	}

	// Process queue
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		// Decrease in-degree for all dependencies
		for dep := range graph[node].Dependencies {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	// Check for cycles
	if len(result) != len(graph) {
		// Find unprocessed nodes for better error message
		unprocessed := make([]string, 0)
		for name := range graph {
			found := false
			for _, processed := range result {
				if name == processed {
					found = true
					break
				}
			}
			if !found {
				unprocessed = append(unprocessed, name)
			}
		}
		return nil, fmt.Errorf("cycle detected in function dependencies involving: %v", unprocessed)
	}

	// The result is in reverse topological order (dependents before dependencies)
	// We need to reverse it to get dependencies before dependents
	slices.Reverse(result)

	return result, nil
}
