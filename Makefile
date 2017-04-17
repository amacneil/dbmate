DC := docker-compose
BUILD_FLAGS := -ldflags '-s'

all: clean container lint test build

clean:
	rm -rf dist

container:
	$(DC) build

lint:
	$(DC) run dbmate golint
	$(DC) run dbmate go vet ./pkg/...
	$(DC) run dbmate errcheck

test:
	$(DC) run dbmate go test -v ./pkg/...

build: clean
	$(DC) run -e GOARCH=386   dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-i386 ./cmd/dbmate
	$(DC) run -e GOARCH=amd64 dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-amd64 ./cmd/dbmate
	# musl target does not support sqlite
	$(DC) run -e GOARCH=amd64 -e CGO_ENABLED=0 dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-musl-amd64 ./cmd/dbmate
