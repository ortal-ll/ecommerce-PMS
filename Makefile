.PHONY: test demo vet

test:
	go test ./... -count=1 -race

demo:
	go run ./cmd/demo/

vet:
	go vet ./...
