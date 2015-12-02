#!/bin/sh
# this script is intended to run inside the docker container
# e.g. docker-compose run dbmate ./build.sh

set -ex

BUILD_FLAGS="-ldflags -s"

rm -rf dist

GOOS=linux GOARCH=386 go build $BUILD_FLAGS \
  -o dist/dbmate-linux-i386
GOOS=linux GOARCH=amd64 go build $BUILD_FLAGS \
  -o dist/dbmate-linux-amd64
GOOS=darwin GOARCH=386 CC=o32-clang CXX=o32-clang++ go build $BUILD_FLAGS \
  -o dist/dbmate-osx-i386
GOOS=darwin GOARCH=amd64 CC=o64-clang CXX=o64-clang++ go build $BUILD_FLAGS \
  -o dist/dbmate-osx-amd64
