.DEFAULT_GOAL := build

.PHONY: fmt vet lint build test check

fmt:
	go fmt ./...

vet: fmt
	go vet ./...

lint: vet
	golangci-lint run ./...

build: lint
	mkdir -p bin
	go tool templ generate
	go build -o bin/faryen ./cmd/server
	go build -o bin/migrate ./cmd/migrate
	go build -o bin/cli ./cmd/cli

test: lint
	go test -race -count=1 -timeout 60s ./...

check: test
