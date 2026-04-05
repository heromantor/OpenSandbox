.PHONY: all build vet test test-integration lint clean fmt check

all: build vet test

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./... -v -short

test-integration:
	go test -tags=integration ./... -v -timeout 3m

lint:
	@which staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed -- skipping lint"

clean:
	go clean ./...
	rm -f coverage.out

fmt:
	gofmt -w .

check: build vet lint
	@echo "All checks passed"
