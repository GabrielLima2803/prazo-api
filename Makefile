# Atalhos de desenvolvimento. Os comandos go equivalentes estão em
# docs/desenvolvimento.md.

.PHONY: build run test test-unit test-integration lint fmt check

build:
	go build -o bin/prazo ./cmd/prazo

run:
	go run ./cmd/prazo serve

test:
	go test -race ./...

# Unitários: ao lado do código.
test-unit:
	go test -race ./internal/... ./cmd/...

# Integração: a API inteira via HTTP, em tests/.
test-integration:
	go test -race ./tests/...

fmt:
	gofmt -w .

lint:
	@test -z "$$(gofmt -l .)" || (echo "arquivos sem gofmt:"; gofmt -l .; exit 1)
	go vet ./...

check: lint test
