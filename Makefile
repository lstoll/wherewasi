GOPATH=$(shell go env GOPATH)

.PHONY: all $(GOPATH)/bin/wherewasi test lint proto docker-dev

go_build_flags=-tags="libsqlite3 sqlite3_unlock_notify"

all: $(GOPATH)/bin/wherewasi test lint

$(GOPATH)/bin/wherewasi:
	go install $(go_build_flags) .

test:
	go test $(go_build_flags) -v .

lint: bin/golangci-lint-1.23.8
	./bin/golangci-lint-1.23.8 run ./...

bin/golangci-lint-1.23.8:
	./hack/fetch-golangci-lint.sh

docker-dev:
	docker buildx build \
		--push \
		--platform linux/arm64/v8,linux/amd64 \
		--tag lstoll/wherewasi:dev .
