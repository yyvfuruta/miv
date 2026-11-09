VERSION := $(shell cat VERSION)

.PHONY: build test check smoke
build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/miv ./cmd/miv
test:
	go test ./...
check:
	go vet ./...
smoke: build
	go run ./scripts/smoke
