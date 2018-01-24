DC := docker-compose
BUILD_FLAGS := -ldflags '-s'
PACKAGES := ./cmd/... ./pkg/...

.PHONY: all
all: dep install test lint build

.PHONY: dep
dep:
	dep ensure -vendor-only

.PHONY: install
install:
	go install -v $(PACKAGES)

.PHONY: test
test:
	go test -v $(PACKAGES)

.PHONY: lint
lint:
	golint -set_exit_status $(PACKAGES)
	go vet $(PACKAGES)
	errcheck $(PACKAGES)

.PHONY: clean
clean:
	rm -rf dist

.PHONY: build
build: clean
	GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/dbmate-linux-amd64 ./cmd/dbmate
	# musl target does not support sqlite
	GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/dbmate-linux-musl-amd64 ./cmd/dbmate

.PHONY: docker
docker:
	$(DC) pull
	$(DC) build
	$(DC) run --rm dbmate make
