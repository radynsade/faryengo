.DEFAULT_GOAL := build

.PHONY: assets templates fmt vet lint build test check

web/admin/assets/node_modules/.package-lock.json: web/admin/assets/package.json web/admin/assets/package-lock.json
	npm ci --prefix web/admin/assets

assets: web/admin/assets/node_modules/.package-lock.json
	npm run build --prefix web/admin/assets

templates: assets
	go tool templ generate

fmt: templates
	go fmt ./...

vet: fmt
	go vet ./...

lint: vet
	npm run format:check --prefix web/admin/assets
	golangci-lint run ./...

build: lint
	rm bin -d -rf
	mkdir -p bin
	go build -o bin/server ./cmd/server
	go build -o bin/migrate ./cmd/migrate
	go build -o bin/cli ./cmd/cli

test: lint
	go test -race -count=1 -timeout 60s ./...

check: test
