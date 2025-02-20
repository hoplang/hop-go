# Run the test suite
test:
	go test -timeout 300ms -coverprofile=coverage.out ./...

# Format code
fmt PATH='.':
	gofumpt -w {{PATH}}
