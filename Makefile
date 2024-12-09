# enable cgo to build sqlite
export CGO_ENABLED = 1

# default output file
OUTPUT ?= dbmate

# platform-specific settings
GOOS := $(shell go env GOOS)
ifeq ($(GOOS),linux)
	# statically link binaries to support alpine linux
	override FLAGS := -tags netgo,osusergo,sqlite_omit_load_extension,sqlite_fts5,sqlite_json -ldflags '-s -extldflags "-static"' $(FLAGS)
else
	# strip binaries
	override FLAGS := -tags sqlite_omit_load_extension,sqlite_fts5,sqlite_json -ldflags '-s' $(FLAGS)
endif
ifeq ($(GOOS),darwin)
	export SDKROOT ?= $(shell xcrun --sdk macosx --show-sdk-path)
endif
ifeq ($(GOOS),windows)
	ifneq ($(suffix $(OUTPUT)),.exe)
		OUTPUT := $(addsuffix .exe,$(OUTPUT))
	endif
endif

.PHONY: all
all: fix build wait test

.PHONY: clean
clean:
	rm -rf dist

.PHONY: build
build: clean
	go build -o dist/$(OUTPUT) $(FLAGS) .

.PHONY: ls
ls:
	ls -lh dist/$(OUTPUT)
	file dist/$(OUTPUT)

.PHONY: test
test:
	go test -v -p 1 $(FLAGS) ./...

.PHONY: lint
lint:
	golangci-lint run --timeout 5m

.PHONY: fix
fix:
	golangci-lint run --fix --timeout 5m

.PHONY: wait
wait:
	dist/dbmate -e CLICKHOUSE_TEST_URL wait
	dist/dbmate -e MYSQL_TEST_URL wait
	dist/dbmate -e POSTGRES_TEST_URL wait

.PHONY: update-deps
update-deps:
	go get -u ./...
	go mod tidy
	go mod verify
	cd typescript && \
		rm -f package-lock.json && \
		./node_modules/.bin/npm-check-updates --upgrade && \
		npm install && \
		npm dedupe

.PHONY: docker-build
docker-build:
	docker compose pull --ignore-buildable
	docker compose build dev

.PHONY: docker-all
docker-all: docker-build
	docker compose run --rm dev make all

.PHONY: docker-sh
docker-sh:
	-docker compose run --rm dev
