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
	$(DC) run dbmate gox --output "dist/{{.Dir}}-{{.OS}}-{{.Arch}}" -cgo -ldflags="-s" -osarch="linux/386" -osarch="linux/amd64" ./cmd/dbmate
	# neither musl (alpine) nor darwin support cgo for sqlite driver because they
	# use different C libraries.
	$(DC) run dbmate gox --output "dist/{{.Dir}}-{{.OS}}-{{.Arch}}-nocgo" -ldflags="-s" -osarch="linux/amd64" -osarch="darwin/amd64" ./cmd/dbmate
