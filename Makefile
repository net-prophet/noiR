GO_LDFLAGS = -ldflags "-s -w"
GO_VERSION = 1.14
GO_TESTPKGS:=$(shell go list ./... | grep -v cmd | grep -v examples)
CI_REGISTRY_IMAGE = docker.netprophet.tech/netp/noir
TEST_REDIS = :6379
REDIS_CLI = docker run -ti --rm --net=host --name noir-redis-cli sameersbn/redis redis-cli

all: docker demo

go_init:
	go mod download
	go generate ./...

protos:
	docker build -t protoc-builder ./pkg/proto && \
	docker run -v $(CURDIR):/workspace protoc-builder protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		--python_out=. \
		pkg/proto/noir.proto

clean:
	rm -rf bin

build: go_init protos
	go build -o bin/noir $(GO_LDFLAGS) ./cmd/noir/main.go

ssl: redis
	go run ./cmd/noir/main.go -c ./config.toml -g :50051 -w :50052 -d :7070 -a :8443 -j :7000 --cert ./cert.pem --key ./cert-key.pem

run: redis
	echo "Running local demo: http://localhost:7070"
	go run ./cmd/noir/main.go -c ./config.toml -g :50051 -w :50052 -d :7070 -a :7777 -j :7000

run_second:
	echo "Running second demo: http://localhost:7071"
	go run ./cmd/noir/main.go -c ./config.toml -d :7069 -j :6999

docker: protos
	docker build . -t ${CI_REGISTRY_IMAGE}:latest

copy_voir:
	cp -r ../voir/build/web/* demo/


tag:
	test ! -z "$$TAG" && (echo "Tagging $$TAG" \
							 && git tag $$TAG  \
							 && git push --tags \
							 && docker build . -t ${CI_REGISTRY_IMAGE}:$$TAG \
							 && docker push ${CI_REGISTRY_IMAGE}:$$TAG \
							 && docker push ${CI_REGISTRY_IMAGE}:latest ) \
		  || echo "usage: make tag TAG=..."

redis:
	docker start noir-redis || docker run -d --rm -p 6379:6379 --name noir-redis sameersbn/redis

redis_cli:
	${REDIS_CLI}

redis_reset:
	${REDIS_CLI} flushdb

redis_all_keys:
	${REDIS_CLI} keys  "noir/*"

redis_room_keys:
	${REDIS_CLI} keys  "noir/obj/room/*"

redis_nodes:
	${REDIS_CLI} hkeys "noir/list/nodes"

redis_user_keys:
	${REDIS_CLI} keys  "noir/obj/user/*"

redis_summary: redis_nodes redis_room_keys redis_user_keys

demo:
	echo "Starting local demonstration at http://localhost:7070" && docker run --net host ghcr.io/net-prophet/noir:latest -d :7070 -j :7000

test: go_init redis_reset redis
	TEST_REDIS=${TEST_REDIS} go test \
		-timeout 120s \
		-coverprofile=cover.out -covermode=atomic \
		-v -race ${GO_TESTPKGS}
