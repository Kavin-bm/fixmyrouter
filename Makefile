BINARY := fixmyrouter
PKG    := ./cmd/fixmyrouter

.PHONY: build test lint run clean

build:
	go build -o bin/$(BINARY) $(PKG)

test:
	go test -race ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

clean:
	rm -rf bin dist
