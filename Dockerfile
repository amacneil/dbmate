# build image
FROM techknowlogick/xgo:go-1.13.x as build
WORKDIR /src
ENTRYPOINT []
CMD ["/bin/bash"]

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
	&& /tmp/lint-install.sh -b /usr/local/bin v1.18.0 \
	&& rm -f /tmp/lint-install.sh

# download modules
COPY go.* ./
RUN go mod download

# build
COPY . ./
RUN make build-linux

# runtime image
FROM gcr.io/distroless/base
COPY --from=build /src/dist/dbmate-linux-amd64 /dbmate
ENTRYPOINT ["/dbmate"]
