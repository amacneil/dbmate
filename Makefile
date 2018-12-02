DC := docker-compose
BUILD_FLAGS := -ldflags '-s'

.PHONY: all
all: test lint build

.PHONY: test
test:
	go test -v ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: wait
wait:
	dist/dbmate-linux-amd64 -e MYSQL_URL wait
	dist/dbmate-linux-amd64 -e POSTGRESQL_URL wait

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
