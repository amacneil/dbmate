FROM golang:1.5.1

ENV CGO_ENABLED 1

# i386 cross compilation
RUN dpkg --add-architecture i386 && \
	apt-get update && \
	apt-get install -y libc6-dev-i386 && \
	rm -rf /var/lib/apt/lists/*

# osx cross compilation
# ref: https://github.com/karalabe/xgo/blob/master/docker/base/Dockerfile
ENV OSX_SDK_FILE MacOSX10.9.sdk.tar.xz
ENV OSX_SDK_SHA256 ac7ccfd8dee95d9811ce50cdc154c8ec7181c51652b5a434445475af62c4fedb
RUN cd /opt && \
	git clone https://github.com/tpoechtrager/osxcross.git && \
	cd osxcross && \
	sed -i -e 's|-march=native||g' ./build_clang.sh ./wrapper/build.sh && \
	apt-get update && \
	./tools/get_dependencies.sh && \
	rm -rf /var/lib/apt/lists/* && \
	curl -fSL -o ./tarballs/$OSX_SDK_FILE \
		https://s3.amazonaws.com/andrew-osx-sdks/$OSX_SDK_FILE && \
	echo "$OSX_SDK_SHA256 ./tarballs/$OSX_SDK_FILE" | sha256sum -c - && \
	UNATTENDED=1 OSX_VERSION_MIN=10.6 ./build.sh
ENV PATH /opt/osxcross/target/bin:$PATH

# development dependencies
RUN go get \
	github.com/golang/lint/golint \
	github.com/kisielk/errcheck \
	golang.org/x/tools/cmd/vet

# copy source files
COPY . $GOPATH/src/github.com/amacneil/dbmate
WORKDIR $GOPATH/src/github.com/amacneil/dbmate

# build
RUN go get -d -t -v
RUN go install -v

CMD dbmate
