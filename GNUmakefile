NAME = vaultwarden
BINARY = terraform-provider-${NAME}

default: fmt lint install generate

build:
	go build -o $(BINARY) -v .

.PHONY: build-ci
build-ci:
	go build -o $(BINARY)

install: build
	go install -v ./...

lint:
	golangci-lint run

generate:
	cd tools; go generate ./...

fmt:
	gofmt -s -w -e .

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

.PHONY: fmt lint test testacc build install generate
