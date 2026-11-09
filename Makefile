.PHONY: build test check smoke
build:
	go build -o bin/miv ./cmd/miv
test:
	go test ./...
check:
	go vet ./...
smoke: build
	python3 scripts/smoke_test.py
