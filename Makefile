.PHONY: check

check:
	go build ./...
	go test -race ./...
	go tool golangci-lint run
	go mod tidy -diff
