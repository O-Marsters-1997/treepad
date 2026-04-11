default:
    @just --list

build:
    go build -o treepad .

run *args:
    go run . {{args}}

test:
    go test ./...

lint:
    golangci-lint run ./...

fmt:
    go fmt ./...

tidy:
    go mod tidy

clean:
    rm -f treepad
