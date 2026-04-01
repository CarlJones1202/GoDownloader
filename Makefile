BINARY_API     := bin/godownload-api
BINARY_MIGRATE := bin/godownload-migrate
CONFIG         := configs/config.yaml

.PHONY: all build api migrate clean run tidy fmt vet test

all: build

build: api migrate

api:
	go build -o $(BINARY_API) ./cmd/api

migrate:
	go build -o $(BINARY_MIGRATE) ./cmd/migrate

run: api
	./$(BINARY_API) -config $(CONFIG)

tidy:
	go mod tidy

fmt:
	gofmt -s -w .

vet:
	go vet ./...

test:
	go test -race ./...

clean:
	rm -rf bin/
