.PHONY: test lint proto run-core run-desk build-core build-desk build-frontend

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

build-frontend:
	cd frontend && npm install && npm run build

build-desk: build-frontend
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
	go build -ldflags "-H windowsgui" -o bin/arb-desk.exe ./cmd/desk
