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
	&& mkdir -p /opt/linux/include /opt/linux/lib \
	&& mkdir -p /opt/darwin/include /opt/darwin/lib \
	&& mkdir -p /opt/win/include /opt/win/lib \
	&& curl -fsSL -o /tmp/orallinux.zip https://download.oracle.com/otn_software/linux/instantclient/19600/instantclient-basic-linux.x64-19.6.0.0.0dbru.zip \
	&& curl -fsSL -o /tmp/oraslinux.zip https://download.oracle.com/otn_software/linux/instantclient/19600/instantclient-sdk-linux.x64-19.6.0.0.0dbru.zip \
	&& curl -fsSL -o /tmp/oraldarwin.zip https://download.oracle.com/otn_software/mac/instantclient/193000/instantclient-basic-macos.x64-19.3.0.0.0dbru.zip \
	&& curl -fsSL -o /tmp/orasdarwin.zip https://download.oracle.com/otn_software/mac/instantclient/193000/instantclient-sdk-macos.x64-19.3.0.0.0dbru.zip \
	&& curl -fsSL -o /tmp/oralwin.zip https://download.oracle.com/otn_software/nt/instantclient/19600/instantclient-basic-windows.x64-19.6.0.0.0dbru.zip \
	&& curl -fsSL -o /tmp/oraswin.zip https://download.oracle.com/otn_software/nt/instantclient/19600/instantclient-sdk-windows.x64-19.6.0.0.0dbru.zip \
	&& unzip /tmp/orallinux.zip -d /tmp/orallinux \
	&& unzip /tmp/oraslinux.zip -d /tmp/oraslinux \
	&& unzip /tmp/oraldarwin.zip -d /tmp/oraldarwin \
	&& unzip /tmp/orasdarwin.zip -d /tmp/orasdarwin \
	&& unzip /tmp/oralwin.zip -d /tmp/oralwin \
	&& unzip /tmp/oraswin.zip -d /tmp/oraswin \
	&& mv /tmp/orallinux/instantclient_19_6/*.so* /opt/linux/lib \
	&& mv /tmp/oraslinux/instantclient_19_6/sdk/include/*.h* /opt/linux/include \
	&& mv /tmp/oraldarwin/instantclient_19_3/*.dylib* /opt/darwin/lib \
	&& mv /tmp/orasdarwin/instantclient_19_3/sdk/include/*.h* /opt/darwin/include \
	&& mv /tmp/oralwin/instantclient_19_6/*.dll /tmp/oralwin/instantclient_19_6/*.sym -t /opt/win/lib \
	&& mv /tmp/oraswin/instantclient_19_6/sdk/include/*.h* /opt/win/include \
	&& rm -rf /var/lib/apt/lists/* \
	&& rm -rf /tmp/ora*

# golangci-lint
RUN curl -fsSL -o /tmp/lint-install.sh https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
	&& chmod +x /tmp/lint-install.sh \
	&& /tmp/lint-install.sh -b /usr/local/bin v1.22.2 \
	&& rm -f /tmp/lint-install.sh

# download modules
COPY go.* ./
RUN go mod download

# build
COPY . ./
ENV PKG_CONFIG_PATH /src/config/linux
ENV LD_LIBRARY_PATH /opt/linux/lib
ENV C_INCLUDE_PATH /opt/linux/include
ENV FORCE_RPATH 1
RUN make build-linux

# runtime image
FROM gcr.io/distroless/base
COPY --from=build /src/dist/dbmate-linux-amd64 /dbmate
COPY --from=build /src/config/linux /config
COPY --from=build /opt/linux /opt/linux
COPY --from=build /lib/x86_64-linux-gnu/libaio* /opt/linux/lib/
ENV PKG_CONFIG_PATH /config
ENV LD_LIBRARY_PATH /opt/linux/lib
ENV C_INCLUDE_PATH /opt/linux/include
ENV FORCE_RPATH 1

ENTRYPOINT ["/dbmate"]
