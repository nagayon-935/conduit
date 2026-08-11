.PHONY: build build-cli build-all run dev tidy test lint

# CGO_ENABLED=0 avoids a macOS dyld LC_UUID linker bug with CGO.
export CGO_ENABLED=0

build:
	go build -o bin/conduit ./cmd/server

build-cli:
	go build -o bin/conduit-cli ./cmd/cli

build-all: build build-cli

run: build
	./bin/conduit

dev:
	go run ./cmd/server

tidy:
	go mod tidy

test:
	go test -race -count=1 ./...

lint:
	go vet ./...
