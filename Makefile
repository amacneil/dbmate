LDFLAGS := -ldflags '-s'

.PHONY: all
all: test lint build

.PHONY: test
test:
	go test -v ./...

.PHONY: fix
fix:
	golangci-lint run --fix

.PHONY: lint
lint:
	golangci-lint run

.PHONY: wait
wait:
	dist/dbmate-linux-amd64 -e MYSQL_URL wait
	dist/dbmate-linux-amd64 -e POSTGRESQL_URL wait

.PHONY: clean
clean:
	rm -rf dist/*

.PHONY: build
build: clean build-linux build-macos build-windows
	ls -lh dist

.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
	     go build $(LDFLAGS) -o dist/dbmate-linux-amd64 .
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
	     go build $(LDFLAGS) -o dist/dbmate-linux-musl-amd64 .

.PHONY: build-macos
build-macos:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 CC=o64-clang CXX=o64-clang++ \
	     go build $(LDFLAGS) -o dist/dbmate-macos-amd64 .

.PHONY: build-windows
build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc-posix CXX=x86_64-w64-mingw32-g++-posix \
	     go build $(LDFLAGS) -o dist/dbmate-windows-amd64.exe .

.PHONY: docker
docker:
	docker-compose build
	docker-compose run --rm dbmate make
