DOCKER := docker-compose run dbmate

all: build lint test

build:
	docker-compose build

lint:
	$(DOCKER) golint ./...
	$(DOCKER) go vet ./...
	$(DOCKER) errcheck ./...

test:
	$(DOCKER) go test -p=1 -v ./...
