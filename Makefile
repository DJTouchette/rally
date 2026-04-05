BINARY := rally
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test lint vet clean install

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/rally

test:
	go test ./... -count=1

lint:
	golangci-lint run ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)

install:
	go install $(LDFLAGS) ./cmd/rally
