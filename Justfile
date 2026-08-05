default:
    @just --list

test:
    go test -race -count=1 ./...

lint:
    golangci-lint run

check: lint test

run:
    go run ./cmd/shipmetrics

build:
    CGO_ENABLED=0 go build -o bin/shipmetrics ./cmd/shipmetrics

fmt:
    gofmt -s -w .
    goimports -w .

tidy:
    go mod tidy

clean:
    rm -rf bin/
