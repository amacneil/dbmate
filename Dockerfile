# development stage
FROM golang:1.27.2 as dev
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
RUN curl -fsSL https://raw.githubusercontent.com/golangci/golangci-lint/main/install.sh \
  | sh -s -- -b /usr/local/bin v2.15.0

# download modules
COPY go.* /src/
RUN go mod download
COPY . /src/
RUN make build

# runtime stage
FROM alpine:3.24.3 as runtime
RUN apk add --no-cache \
  mariadb-client \
  mariadb-connector-c \
  postgresql-client \
  sqlite \
  tzdata
ENTRYPOINT ["/usr/local/bin/dbmate"]

# release stage
FROM runtime as release
COPY --from=dev /src/dist/dbmate /usr/local/bin/dbmate

# release stage using binaries from the CI build matrix
FROM runtime as release-prebuilt
ARG TARGETARCH
COPY --chmod=0755 dist/dbmate-linux-${TARGETARCH} /usr/local/bin/dbmate
