FROM golang:1.6.2

ENV CGO_ENABLED 1

# i386 cross compilation
RUN dpkg --add-architecture i386 && \
	apt-get update && \
	apt-get install -y libc6-dev-i386 && \
	rm -rf /var/lib/apt/lists/*

# development dependencies
RUN go get \
	github.com/golang/lint/golint \
	github.com/kisielk/errcheck

# copy source files
COPY . $GOPATH/src/github.com/amacneil/dbmate
WORKDIR $GOPATH/src/github.com/amacneil/dbmate

# build
RUN go get -d -t -v
RUN go install -v

CMD dbmate
