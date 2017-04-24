DC := docker-compose
BUILD_FLAGS := -ldflags '-s'

all: clean container lint test build

clean:
	rm -rf dist

container:
	$(DC) pull
	$(DC) build
	$(DC) up -d

lint:
	$(DC) run dbmate golint
	$(DC) run dbmate go vet
	$(DC) run dbmate errcheck

test:
	$(DC) run dbmate go test -v

build: clean
	$(DC) run -e GOARCH=386   dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-i386
	$(DC) run -e GOARCH=amd64 dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-amd64
	# musl target does not support sqlite
	$(DC) run -e GOARCH=amd64 -e CGO_ENABLED=0 dbmate go build $(BUILD_FLAGS) -o dist/dbmate-linux-musl-amd64
