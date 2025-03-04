package toposort

import (
	"fmt"
	"slices"
)

// TopologicalSort runs Kahn's algorithm on the given dependency graph.
func TopologicalSort(graph map[string]map[string]bool, label string) ([]string, error) {
	inDegree := make(map[string]int)
	for node, dependencies := range graph {
		if _, ok := inDegree[node]; !ok {
			inDegree[node] = 0
		}
		for dep := range dependencies {
			if _, exists := graph[dep]; !exists {
				return nil, fmt.Errorf("%s '%s' depends on undefined %s '%s'", label, node, label, dep)
			}
			inDegree[dep]++
		}
	}
	var queue []string
	for node, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, node)
		}
	}
	var result []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)
		for dep := range graph[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}
	if len(result) != len(graph) {
		unprocessed := make([]string, 0)
		processedSet := make(map[string]bool, len(result))
		for _, v := range result {
			processedSet[v] = true
		}
		for node := range graph {
			if !processedSet[node] {
				unprocessed = append(unprocessed, node)
			}
		}
		return nil, fmt.Errorf("cycle detected in dependencies involving: %v", unprocessed)
	}
	slices.Reverse(result)
	return result, nil
}
