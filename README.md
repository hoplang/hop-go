<img src=".github/cover.svg">

-------------------------------------------------------------------------------

* [Go package documentation](https://pkg.go.dev/github.com/hoplang/hop-go)

Quickstart
-------------------------------------------------------------------------------

* `go get github.com/hoplang/hop-go`

```go
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/hoplang/hop-go"
)

const template = `
<function name="main" params-as="items">
	<!DOCTYPE html>
	<html>
	<head>
		<title>Quickstart</title>
	</head>
	<body>
		<for each="items" as="item">
			<div inner-text="item.title"></div>
		</for>
	</body>
	</html>
</function>
`

type Item struct {
	Title string `json:"title"`
}

func main() {
	compiler := hop.NewCompiler()
	compiler.AddModule("main", template)
	program, err := compiler.Compile()
	if err != nil {
		log.Fatalf("Failed to compile: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		data := []Item{
			{Title: "foo"},
			{Title: "bar"},
			{Title: "baz"},
		}
		err := program.ExecuteFunction(w, "main", "main", data)
		if err != nil {
			log.Fatalf("Failed to execute: %v", err)
		}
	})
	fmt.Println("Server starting at http://localhost:8089")
	err = http.ListenAndServe(":8089", mux)
	if err != nil {
		log.Fatalf("Failed to compile: %v", err)
	}
}
```
