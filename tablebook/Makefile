.PHONY: run test race vet fmt docker docker-run

run:
	go run ./cmd/server

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

docker:
	docker build -t tablebook .

docker-run: docker
	docker run --rm -p 8080:8080 tablebook
