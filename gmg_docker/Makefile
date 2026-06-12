APP_NAME ?= go-mini-gateway
CONFIG ?= configs/gateway.json
ADMIN_TOKEN ?= dev-token
GO_VERSION ?= 1.26
ALPINE_MIRROR ?= https://mirrors.aliyun.com/alpine
GOPROXY ?= https://goproxy.cn,direct

.PHONY: test build run-gateway run-backend1 run-backend2 docker-build docker-up docker-down docker-logs docker-ps docker-reload docker-smoke clean

test:
	go test ./...

build:
	mkdir -p bin
	go build -o bin/gateway ./cmd/gateway
	go build -o bin/backend ./cmd/backend
	go build -o bin/backend2 ./cmd/backend2

run-gateway:
	GATEWAY_ADMIN_TOKEN=$(ADMIN_TOKEN) go run ./cmd/gateway -config $(CONFIG)

run-backend1:
	go run ./cmd/backend

run-backend2:
	go run ./cmd/backend2

docker-build:
	GO_VERSION=$(GO_VERSION) ALPINE_MIRROR=$(ALPINE_MIRROR) GOPROXY=$(GOPROXY) docker compose build

docker-up:
	GATEWAY_ADMIN_TOKEN=$(ADMIN_TOKEN) GO_VERSION=$(GO_VERSION) ALPINE_MIRROR=$(ALPINE_MIRROR) GOPROXY=$(GOPROXY) docker compose up --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

docker-ps:
	docker compose ps

docker-reload:
	curl -X POST -H "X-Admin-Token: $(ADMIN_TOKEN)" http://127.0.0.1:9001/admin/reload

docker-smoke:
	curl -i http://127.0.0.1:9000/ping
	curl -i http://127.0.0.1:9000/api/hello?name=docker
	curl -i -H "X-Admin-Token: $(ADMIN_TOKEN)" http://127.0.0.1:9001/admin/routes
	curl -s http://127.0.0.1:9001/metrics | head

clean:
	rm -rf bin coverage.out
