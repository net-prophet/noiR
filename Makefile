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

tag:
	test ! -z "$$TAG" && (echo "Tagging $$TAG" \
							 && git tag $$TAG  \
							 && git push --tags \
							 && docker build . -t ${CI_REGISTRY_IMAGE}:$$TAG \
							 && docker push ${CI_REGISTRY_IMAGE}:$$TAG \
							 && docker push ${CI_REGISTRY_IMAGE}:latest ) \
		  || echo "usage: make tag TAG=..."

demo_redis:
	docker run -p 6379:6379 --name redis sameersbn/redis redis-cli

readme_demo:
	echo "Starting local demonstration at http://localhost:7070" && docker run --net host ghcr.io/net-prophet/noir:latest -d :7070 -j :7000

test: go_init
	go test \
		-timeout 120s \
		-coverprofile=cover.out -covermode=atomic \
		-v -race ${GO_TESTPKGS}
