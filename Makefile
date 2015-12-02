DOCKER := docker-compose run dbmate

all: container lint test build

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
