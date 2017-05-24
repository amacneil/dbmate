DC := docker-compose
BUILD_FLAGS := -ldflags '-s'
PACKAGES := . ./cmd/dbmate

all: clean container test lint build

clean:
	rm -rf dist

container:
	$(DC) pull
	$(DC) build
	$(DC) up -d

lint:
	$(DC) run dbmate python lint-gofmt.py
	$(DC) run dbmate golint -set_exit_status $(PACKAGES)
	$(DC) run dbmate go vet $(PACKAGES)
	$(DC) run dbmate errcheck $(PACKAGES)

test:
	$(DC) run dbmate go test -v $(PACKAGES)

build: clean
	$(DC) run -e GOARCH=386   dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-i386 ./cmd/dbmate
	$(DC) run -e GOARCH=amd64 dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-amd64 ./cmd/dbmate
	# musl target does not support sqlite
	$(DC) run -e GOARCH=amd64 -e CGO_ENABLED=0 dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-musl-amd64 ./cmd/dbmate
