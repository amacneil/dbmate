# enable cgo to build sqlite
export CGO_ENABLED = 1

# strip binaries
FLAGS := -tags sqlite_omit_load_extension,sqlite_json -ldflags '-s'

# default output file
OUTPUT ?= dbmate

# platform-specific settings
GOOS := $(shell go env GOOS)
ifeq ($(GOOS),linux)
	# statically link binaries to support alpine linux
	FLAGS := -tags netgo,osusergo,sqlite_omit_load_extension,sqlite_json  -ldflags '-s -extldflags "-static"'
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
	go test -p 1 $(FLAGS) ./...

.PHONY: lint
lint:
	golangci-lint run --timeout 2m

.PHONY: fix
fix:
	golangci-lint run --fix

.PHONY: wait
wait:
	dist/dbmate -e CLICKHOUSE_TEST_URL wait
	dist/dbmate -e MYSQL_TEST_URL wait
	dist/dbmate -e POSTGRES_TEST_URL wait

.PHONY: docker-all
docker-all:
	docker-compose pull
	docker-compose build
	docker-compose run --rm dev make all

.PHONY: docker-sh
docker-sh:
	-docker-compose run --rm dev
