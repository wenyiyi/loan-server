-include .env
export DATABASE_URL

.PHONY: run worker test integration vet build
run:
	go run ./cmd/server
test:
	go test -race ./...
integration:
	test -n "$$DATABASE_URL"
	go test -tags=integration -race -count=1 ./internal/model ./tests/integration -v
vet:
	go vet ./...
build:
	go build -o bin/server ./cmd/server
	go build -o bin/worker ./cmd/worker

worker:
	go run ./cmd/worker
