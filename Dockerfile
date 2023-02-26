# development image
FROM golang:1.20 as dev
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
	| sh -s -- -b /usr/local/bin v1.51.2

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
ENTRYPOINT ["/usr/local/bin/dbmate"]
