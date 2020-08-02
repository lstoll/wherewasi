GOPATH=$(shell go env GOPATH)

.PHONY: all $(GOPATH)/bin/wherewasi test lint proto orion-index.html

go_build_flags="-tags=libsqlite3"
orion_ref="3c4d396989ea2f7ee61a9a15ce745e322343001f"

all: $(GOPATH)/bin/wherewasi test lint

$(GOPATH)/bin/wherewasi:
	go install $(go_build_flags) .

test:
	go test $(go_build_flags) -v .

lint: bin/golangci-lint-1.23.8
	./bin/golangci-lint-1.23.8 run ./...

bin/golangci-lint-1.23.8:
	./hack/fetch-golangci-lint.sh

orion-index.html:
	$(eval BUILDTMP := $(shell mktemp -d))
	cd $(BUILDTMP) && git clone https://github.com/LINKIWI/orion-web.git
	cd $(BUILDTMP)/orion-web && git checkout $(orion_ref)
	cd $(BUILDTMP)/orion-web && npm install
	cd $(BUILDTMP)/orion-web && NODE_ENV=production MAPBOX_API_TOKEN='{{ .MapboxAPIToken }}' npm run build
	cp $(BUILDTMP)/orion-web/dist/index.html orion-index.html
