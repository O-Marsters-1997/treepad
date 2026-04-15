default:
    @just --list

build:
    go build -o treepad ./cmd/treepad

run *args:
    go run ./cmd/treepad/main.go {{args}}

test:
    go test ./...

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
    rm -f treepad

ci:
    golangci-lint run ./...
    just build
    just test