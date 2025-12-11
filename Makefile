.PHONY: test lint

test:
	go test -v -race -coverprofile=coverage.out ./...

lint:
	namedreturns ./...
	golangci-lint run
