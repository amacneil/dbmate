# Dbmate

## Installation

Dbmate is currently under development. To install the latest build, run:

```sh
$ go get -u github.com/adrianmacneil/dbmate
```

## Testing

Tests are run with docker-compose. First, install the [Docker Toolbox](https://www.docker.com/docker-toolbox).

Make sure you have docker running:

```sh
$ docker-machine start default && eval "$(docker-machine env default)"
```

To build a docker image and run the tests:

```sh
$ make
```
