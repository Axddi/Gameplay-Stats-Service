.PHONY: test run build docker

test:
	go vet ./...
	go test -race -cover ./...

run:
	go run .

build:
	go build -o bin/server .

docker:
	docker build -t gameplay-stats-service .
