GOPATH=$(shell go env GOPATH)

.PHONY: all $(GOPATH)/bin/wherewasi test lint proto

go_build_flags=

all: $(GOPATH)/bin/wherewasi test lint

$(GOPATH)/bin/wherewasi:
	go install $(go_build_flags) .

test:
	go test $(go_build_flags) -v .

lint: bin/golangci-lint-1.23.8
	./bin/golangci-lint-1.23.8 run ./...

bin/golangci-lint-1.23.8:
	./hack/fetch-golangci-lint.sh
