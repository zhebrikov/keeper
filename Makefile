.PHONY: build build-client build-server proto test coverage lint run-server tls-certs doc swagger

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -X github.com/zhebrikov/gophkeeper/internal/version.Version=$(VERSION) \
          -X github.com/zhebrikov/gophkeeper/internal/version.BuildDate=$(BUILD_DATE)

PROTOC := PATH="$(shell go env GOPATH)/bin:/opt/homebrew/bin:$$PATH" protoc

# Exclude generated code, main packages, and CLI bootstrap wrappers from coverage.
COVER_PACKAGES := $(shell go list ./... | grep -v '/api/gen/' | grep -v '/cmd/' | grep -v '/internal/app/')

# Packages checked for godoc completeness (excludes generated protobuf).
DOC_PACKAGES := $(shell go list ./... | grep -v '/api/gen/')

build: build-client build-server

build-client:
	go build -ldflags "$(LDFLAGS)" -o bin/gophkeeper-client ./cmd/client

swagger: proto

build-server: swagger
	go build -o bin/gophkeeper-server ./cmd/server

proto:
	@mkdir -p internal/swagger/openapi
	$(PROTOC) --go_out=api/gen --go_opt=paths=source_relative \
		--go-grpc_out=api/gen --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=api/gen --grpc-gateway_opt=paths=source_relative \
		--openapiv2_out=internal/swagger/openapi --openapiv2_opt=logtostderr=true,allow_merge=true,merge_file_name=gophkeeper \
		-I api/proto api/proto/gophkeeper/v1/gophkeeper.proto

test:
	go test $(COVER_PACKAGES) -count=1

coverage:
	go test $(COVER_PACKAGES) -coverprofile=coverage.out -count=1
	@pct=$$(go tool cover -func=coverage.out | awk '/total:/ {print $$3}' | tr -d '%'); \
	echo "total: $$pct%"; \
	awk "BEGIN {exit !($$pct >= 70)}"

run-server: tls-certs
	TLS_CERT=certs/server.crt TLS_KEY=certs/server.key go run ./cmd/server

tls-certs:
	@mkdir -p certs
	@if [ ! -f certs/server.crt ]; then \
		openssl req -x509 -newkey rsa:4096 -keyout certs/server.key -out certs/server.crt -days 365 -nodes -subj "/CN=localhost"; \
		cp certs/server.crt certs/ca.crt; \
	fi

lint:
	go vet ./...
	@command -v staticcheck >/dev/null 2>&1 || go install honnef.co/go/tools/cmd/staticcheck@latest
	staticcheck -checks=ST1000,ST1020 $(DOC_PACKAGES)

doc:
	@echo "Documentation server: http://localhost:6060/pkg/github.com/zhebrikov/gophkeeper/"
	go doc -http=:6060
