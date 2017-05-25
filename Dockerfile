FROM golang:1.8

# i386 cross compilation
RUN dpkg --add-architecture i386 && \
	apt-get update && \
	apt-get install -y libc6-dev-i386 && \
	rm -rf /var/lib/apt/lists/*

# development dependencies
RUN go get \
	github.com/mitchellh/gox \
	github.com/golang/lint/golint \
	github.com/kisielk/errcheck

# copy source files
COPY . $GOPATH/src/github.com/turnitin/dbmate
WORKDIR $GOPATH/src/github.com/turnitin/dbmate

# build
RUN go install -v ./cmd/dbmate

CMD dbmate
