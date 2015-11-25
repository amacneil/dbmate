FROM alpine:3.2

ENV GOPATH /go
ENV PATH /go/bin:$PATH

# install build dependencies
RUN apk add -U --no-progress go git ca-certificates
RUN go get \
	github.com/golang/lint/golint \
	golang.org/x/tools/cmd/vet

# copy source files
COPY . /go/src/github.com/adrianmacneil/dbmate
WORKDIR /go/src/github.com/adrianmacneil/dbmate

# build
RUN go get -d -t
RUN go install -v

CMD dbmate
