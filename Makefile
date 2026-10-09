.PHONY: run build test lint tidy clean

run:
	set -a; . ./.env; set +a; \
	POSTGRES_HOST=127.0.0.1 go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

test:
	set -a; . ./.env; set +a; \
	POSTGRES_HOST=127.0.0.1 go test ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -rf bin/