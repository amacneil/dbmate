# development image
FROM golang:1.21 as dev
WORKDIR /src
RUN git config --global --add safe.directory /src

# install database clients
RUN apt-get update \
	&& apt-get install -qq --no-install-recommends \
		curl \
		file \
		mariadb-client \
		postgresql-client \
		sqlite3 \
	&& rm -rf /var/lib/apt/lists/*

# golangci-lint
RUN curl -fsSL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
	| sh -s -- -b /usr/local/bin v1.54.2

# libsql-shell-go
RUN go install github.com/libsql/libsql-shell-go/cmd/libsql-shell@latest

# download modules
COPY go.* /src/
RUN go mod download
COPY . /src/
RUN make build

# release stage
FROM alpine as release
RUN apk add --no-cache \
	mariadb-client \
	mariadb-connector-c \
	postgresql-client \
	sqlite \
	tzdata
COPY --from=dev /src/dist/dbmate /usr/local/bin/dbmate
COPY --from=dev /go/bin/libsql-shell /usr/local/bin/libsql-shell
ENTRYPOINT ["/usr/local/bin/dbmate"]
