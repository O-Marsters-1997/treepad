default:
    @just --list

build:
    go build -o tp ./cmd/tp

run *args:
    go run ./cmd/tp/main.go {{args}}

test:
    go test ./...

test-e2e:
    go test -tags=e2e ./e2e/...

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

check-conflicts:
    #!/usr/bin/env bash
    branch=$(git rev-parse --abbrev-ref HEAD)
    if [ "$branch" = "main" ]; then exit 0; fi
    base=$(git merge-base HEAD origin/main 2>/dev/null)
    if git merge-tree "$base" HEAD origin/main 2>/dev/null | grep -q "^<<<<<<< "; then
        echo "error: branch has conflicts with origin/main" >&2 && exit 1
    fi

ci:
    just check-conflicts
    just build
    golangci-lint run ./...
    just test
    just test-e2e