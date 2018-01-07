FROM golang:1.9

# required to force cgo (for sqlite driver) with cross compile
ENV CGO_ENABLED 1

# development dependencies
RUN go get \
	github.com/golang/lint/golint \
	github.com/kisielk/errcheck

# copy source files
COPY . $GOPATH/src/github.com/amacneil/dbmate
WORKDIR $GOPATH/src/github.com/amacneil/dbmate

# build
RUN go install -v ./cmd/dbmate

CMD dbmate
