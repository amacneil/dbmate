# build image
FROM golang:1.10 as build

# required to force cgo (for sqlite driver) with cross compile
ENV CGO_ENABLED 1

# install database clients
RUN apt-get update \
	&& apt-get install -y --no-install-recommends \
		mysql-client \
		postgresql-client \
		sqlite3 \
	&& rm -rf /var/lib/apt/lists/*

# development dependencies
RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/v0.3.2/dep-linux-amd64 \
	&& chmod +x /usr/local/bin/dep
RUN go get gopkg.in/alecthomas/gometalinter.v2 \
	&& gometalinter.v2 --install

# copy source files
COPY . /go/src/github.com/amacneil/dbmate
WORKDIR /go/src/github.com/amacneil/dbmate

# build
RUN make dep install build

# runtime image
FROM debian:stretch-slim
COPY --from=build /go/src/github.com/amacneil/dbmate/dist/dbmate-linux-amd64 \
	/usr/local/bin/dbmate
WORKDIR /app
ENTRYPOINT ["/usr/local/bin/dbmate"]
