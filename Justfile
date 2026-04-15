default:
    @just --list

build:
    go build -o tp ./cmd/tp

run *args:
    go run ./cmd/tp/main.go {{args}}

test:
    go test ./...

test-e2e:
    go test -tags=e2e ./cmd/tp/...

lint:
    docker run --rm \
        -v "$(pwd):/app" \
        -v treepad-golangci-lint-cache:/root/.cache \
        -v treepad-go-mod-cache:/root/go/pkg/mod \
        -w /app \
        golangci/golangci-lint:latest \
        golangci-lint run ./...

fmt:
    go fmt ./...

tidy:
    go mod tidy

clean:
    rm -f tp

ci:
    golangci-lint run ./...
    just build
    just test
    just test-e2e