# build image
FROM techknowlogick/xgo:go-1.14.x as build
WORKDIR /src

# enable cgo to build sqlite
ENV CGO_ENABLED 1

# install database clients
RUN apt-get update \
	&& apt-get install -qq --no-install-recommends \
		curl \
		mysql-client \
		postgresql-client \
		sqlite3 \
	&& rm -rf /var/lib/apt/lists/*

# golangci-lint
RUN curl -fsSL -o /tmp/lint-install.sh https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
	&& chmod +x /tmp/lint-install.sh \
	&& /tmp/lint-install.sh -b /usr/local/bin v1.30.0 \
	&& rm -f /tmp/lint-install.sh

# download modules
COPY go.* ./
RUN go mod download

# build
COPY . ./
RUN make build

ENTRYPOINT []
CMD ["/bin/bash"]

# runtime image
FROM alpine
RUN apk add --no-cache \
        mariadb-client \
        postgresql-client \
        sqlite
COPY --from=build /src/dist/dbmate-linux-amd64 /usr/local/bin/dbmate
ENTRYPOINT ["dbmate"]
