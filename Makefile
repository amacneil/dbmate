DOCKER := docker-compose run dbmate

all: clean container lint test build

clean:
	rm -rf dist

container:
	docker-compose build

lint:
	$(DOCKER) golint
	$(DOCKER) go vet
	$(DOCKER) errcheck

test:
	$(DOCKER) go test -v

build:
	docker-compose run dbmate ./build.sh
