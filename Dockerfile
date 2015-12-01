FROM alpine:edge

ENV GOPATH /go
ENV PATH /go/bin:$PATH

# install build dependencies
RUN echo "http://dl-4.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories && \
	apk add -U --no-progress alpine-sdk go go-tools
RUN go get \
	github.com/golang/lint/golint \
	github.com/kisielk/errcheck \
	golang.org/x/tools/cmd/vet

# copy source files
COPY . /go/src/github.com/adrianmacneil/dbmate
WORKDIR /go/src/github.com/adrianmacneil/dbmate

# build
RUN go get -d -t -v
RUN go install -v

CMD dbmate
