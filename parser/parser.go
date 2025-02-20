package parser

import (
	"strings"

	"golang.org/x/net/html"
)

func Parse(template string) (*html.Node, error) {
	root := &html.Node{
		Type: html.ElementNode,
		Data: "root",
	}
	nodes, err := html.ParseFragment(strings.NewReader(template), root)
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	return root, nil
}
