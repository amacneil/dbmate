DC := docker-compose
BUILD_FLAGS := -ldflags '-s'
PACKAGES := . ./pkg/...

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
	gometalinter.v2 $(PACKAGES)

.PHONY: clean
clean:
	rm -rf dist

.PHONY: build
build: clean
	GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/dbmate-linux-amd64 .
	# musl target does not support sqlite
	GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/dbmate-linux-musl-amd64 .

.PHONY: docker
docker:
	$(DC) pull
	$(DC) build
	$(DC) run --rm dbmate make
