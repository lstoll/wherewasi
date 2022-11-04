GOPATH=$(shell go env GOPATH)

.PHONY: all $(GOPATH)/bin/wherewasi test lint proto docker-dev

go_build_flags=

all: $(GOPATH)/bin/wherewasi test lint

$(GOPATH)/bin/wherewasi:
	go install $(go_build_flags) .

test:
	go test $(go_build_flags) -v .

lint:
	golangci-lint run ./...
