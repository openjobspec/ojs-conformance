.PHONY: build test lint fmt

build:
	go build ./...

test:
	go build ./...

lint:
	go vet ./...

fmt:
	gofmt -w .
