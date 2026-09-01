.PHONY: all build test vet lint clean fmt docker

APP_NAME    := breviary
GO_FLAGS    := -ldflags="-s -w"
TEST_FLAGS  := -count=1 -race

all: clean fmt vet test build lint

build:
	go build $(GO_FLAGS) -o $(APP_NAME) ./cmd/breviary/

test:
	go test $(TEST_FLAGS) ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

clean:
	rm -f $(APP_NAME) $(APP_NAME).db

docker:
	docker build -t ghcr.io/wootwooty/$(APP_NAME):latest .

docker-slim:
	docker build -t ghcr.io/wootwooty/$(APP_NAME):slim -f Dockerfile.slim .

pre-commit: fmt vet test lint

.PHONY: all build test vet lint clean fmt docker docker-slim pre-commit