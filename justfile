# Run the test suite
test:
	go test -coverprofile=coverage.out ./...

# Format code
fmt PATH='.':
	gofumpt -w {{PATH}}
