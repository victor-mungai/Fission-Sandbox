.PHONY: build run test fmt

build:
	go build -o bin/api ./cmd/api

run:
	go run ./cmd/api

test:
	go test ./...

fmt:
	go fmt ./...
