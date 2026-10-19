.PHONY: run worker test integration vet build wire
run:
	go run ./cmd/server
test:
	go test -race ./...
integration:
	go test -tags=integration -race -count=1 ./internal/model ./tests/integration -v
vet:
	go vet ./...
build:
	go build -o bin/server ./cmd/server
	go build -o bin/worker ./cmd/worker

worker:
	go run ./cmd/worker

wire:
	go generate ./cmd/server/wire ./cmd/worker/wire
