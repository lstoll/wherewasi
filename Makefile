.PHONY: all test lint proto

GOPATH=$(shell go env GOPATH)

all: $(GOPATH)/bin/wherewasi test lint

$(GOPATH)/bin/wherewasi: *
	go install .

test:
	go test -v .

lint: bin/golangci-lint-1.23.8
	./bin/golangci-lint-1.23.8 run ./...

bin/golangci-lint-1.23.8:
	./hack/fetch-golangci-lint.sh
