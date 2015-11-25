DOCKER := docker-compose run dbmate

all: build lint test

build:
	docker-compose build

lint:
	$(DOCKER) golint ./...
	$(DOCKER) go vet ./...

test:
	$(DOCKER) go test ./...
