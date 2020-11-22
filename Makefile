GO_LDFLAGS = -ldflags "-s -w"
GO_VERSION = 1.14
GO_TESTPKGS:=$(shell go list ./... | grep -v cmd | grep -v examples)
CI_REGISTRY_IMAGE = ghcr.io/net-prophet/noir

all: build

go_init:
	go mod download
	go generate ./...

clean:
	rm -rf bin

build: go_init
	go build -o bin/noir $(GO_LDFLAGS) ./cmd/noir/main.go

docker:
	docker build . -t ${CI_REGISTRY_IMAGE}:latest

test: go_init
	go test \
		-timeout 120s \
		-coverprofile=cover.out -covermode=atomic \
		-v -race ${GO_TESTPKGS}
