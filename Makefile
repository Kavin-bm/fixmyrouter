BINARY := fixmyrouter
PKG    := ./cmd/fixmyrouter

.PHONY: build test lint run clean

build:
	go build -o bin/$(BINARY) $(PKG)

test:
	go test ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)

clean:
	rm -rf bin dist
