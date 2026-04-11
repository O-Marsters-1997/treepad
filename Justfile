default:
    @just --list

build:
    go build -o treepad .

run *args:
    go run . {{args}}

test:
    go test ./...

tidy:
    go mod tidy

clean:
    rm -f treepad
