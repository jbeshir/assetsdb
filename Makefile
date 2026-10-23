# assetsdb Makefile
# Single-module repo: builds and validates github.com/jbeshir/assetsdb.

.PHONY: setup-tools
setup-tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0

# ── Validation ────────────────────────────────────────────────

.PHONY: validate
validate: lint test build

.PHONY: lint
lint:
	golangci-lint run --config .golangci.yml ./...

.PHONY: test
test:
	go test ./...

.PHONY: test-short
test-short:
	go test -short ./...

.PHONY: fmt
fmt:
	gofmt -w .

# ── Build ─────────────────────────────────────────────────────

.PHONY: build
build:
	go build -o bin/assetsdb ./cmd/assetsdb

# ── Clean ─────────────────────────────────────────────────────

.PHONY: clean
clean:
	rm -rf bin/
