run:
	go run cmd/main.go main .

test:
	go test -v ./...

build:
	go build -o grep-ast cmd/main.go