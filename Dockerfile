# development image
FROM techknowlogick/xgo:go-1.16.x as dev
WORKDIR /src
ENV GOCACHE /src/.cache/go-build

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
RUN curl -fsSL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
	| sh -s -- -b /usr/local/bin v1.39.0

# download modules
COPY go.* ./
RUN go mod download

ENTRYPOINT []
CMD ["/bin/bash"]

# build stage
FROM dev as build
COPY . ./
RUN make build

# release stage
FROM alpine as release
RUN apk add --no-cache \
	mariadb-client \
	postgresql-client \
	sqlite \
	tzdata
COPY --from=build /src/dist/dbmate-linux-amd64 /usr/local/bin/dbmate
ENTRYPOINT ["dbmate"]
