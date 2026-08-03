.PHONY: test lint proto run-core run-desk build-core build-desk

proto:
	buf generate proto/

test:
	go test -race -count=1 $(go list ./... | grep -v '/desk')

lint:
	go vet $(go list ./... | grep -v '/desk')

run-core:
	go run ./cmd/core -config=config/default.textproto

run-desk:
	go run ./cmd/desk

build-core:
	CGO_ENABLED=0 go build -o bin/arb-core ./cmd/core

build-desk:
	go build -o bin/arb-desk ./cmd/desk
