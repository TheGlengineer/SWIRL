# Development container for SWIRL: the Dreamcast toolchain (GCC 15.1 + KallistiOS 2.2.1), Go and the
# tools the build scripts use.
FROM mcr.microsoft.com/devcontainers/base:ubuntu-22.04

RUN apt-get update \
 && apt-get install -y --no-install-recommends build-essential curl ca-certificates python3 python3-pil \
    binutils-mingw-w64-x86-64 \
 && rm -rf /var/lib/apt/lists/*

COPY swirl/setup_toolchain.sh /tmp/setup_toolchain.sh
RUN bash /tmp/setup_toolchain.sh /opt/dreamcast && rm /tmp/setup_toolchain.sh
ENV KOS_ENV=/opt/dreamcast/kos/environ.sh

ARG GO_VERSION=1.22.7
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -xz -C /usr/local
ENV PATH=/usr/local/go/bin:$PATH
