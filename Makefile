DC := docker-compose
BUILD_FLAGS := -ldflags '-s'
PACKAGES := ./cmd/... ./pkg/...

all: clean container test lint build

clean:
	rm -rf dist

container:
	$(DC) pull
	$(DC) build
	$(DC) up -d

lint:
	$(DC) run --rm dbmate golint -set_exit_status $(PACKAGES)
	$(DC) run --rm dbmate go vet $(PACKAGES)
	$(DC) run --rm dbmate errcheck $(PACKAGES)

test:
	$(DC) run --rm dbmate go test -v $(PACKAGES)

build: clean
	$(DC) run --rm -e GOARCH=amd64 dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-amd64 ./cmd/dbmate
	# musl target does not support sqlite
	$(DC) run --rm -e GOARCH=amd64 -e CGO_ENABLED=0 dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-musl-amd64 ./cmd/dbmate
