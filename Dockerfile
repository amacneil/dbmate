# development stage
FROM golang:1.25.6 as dev
WORKDIR /src
ENV PATH="/src/typescript/node_modules/.bin:${PATH}"
RUN git config --global --add safe.directory /src

# install development tools
RUN apt-get update \
  && apt-get install -qq --no-install-recommends \
    curl \
    file \
    mariadb-client \
    postgresql-client \
    sqlite3 \
    nodejs \
    npm \
  && rm -rf /var/lib/apt/lists/*

# golangci-lint
RUN curl -fsSL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b /usr/local/bin v2.8.0

# download modules
COPY go.* /src/
RUN go mod download
COPY . /src/
RUN make build

# release stage
FROM alpine:3.23.3 as release
RUN apk add --no-cache \
  mariadb-client \
  mariadb-connector-c \
  postgresql-client \
  sqlite \
  tzdata
COPY --from=dev /src/dist/dbmate /usr/local/bin/dbmate
ENTRYPOINT ["/usr/local/bin/dbmate"]
