.PHONY: build run test fmt

GOCACHE ?= $(CURDIR)/.gocache

build:
	GOCACHE=$(GOCACHE) go build -o bin/api ./cmd/api

run:
	GOCACHE=$(GOCACHE) go run ./cmd/api

test:
	GOCACHE=$(GOCACHE) go test ./...

fmt:
	GOCACHE=$(GOCACHE) go fmt ./...
