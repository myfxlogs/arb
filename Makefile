.PHONY: test lint proto run-core build-core

proto:
	buf generate proto/

test:
	go test -race -count=1 ./...

lint:
	go vet ./...

run-core:
	go run ./cmd/core -config=config/default.textproto

build-core:
	CGO_ENABLED=0 go build -o bin/arb-core ./cmd/core
