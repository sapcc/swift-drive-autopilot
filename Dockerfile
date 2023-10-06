FROM golang:1.21.2-alpine3.18 as builder

RUN apk add --no-cache --no-progress gcc git make musl-dev

COPY . /src
ARG BININFO_BUILD_DATE BININFO_COMMIT_HASH BININFO_VERSION # provided to 'make install'
RUN make -C /src install PREFIX=/pkg GOTOOLCHAIN=local GO_BUILDFLAGS='-mod vendor'

################################################################################

FROM alpine:3.18

# upgrade all installed packages to fix potential CVEs in advance
RUN apk upgrade --no-cache --no-progress \
  && apk add --no-cache --no-progress ca-certificates dumb-init file smartmontools

COPY --from=builder /pkg/ /usr/

ARG BININFO_BUILD_DATE BININFO_COMMIT_HASH BININFO_VERSION
LABEL source_repository="https://github.com/sapcc/swift-drive-autopilot" \
  org.opencontainers.image.url="https://github.com/sapcc/swift-drive-autopilot" \
  org.opencontainers.image.created=${BININFO_BUILD_DATE} \
  org.opencontainers.image.revision=${BININFO_COMMIT_HASH} \
  org.opencontainers.image.version=${BININFO_VERSION}

WORKDIR /
ENTRYPOINT [ "/usr/bin/dumb-init", "--", "/usr/bin/swift-drive-autopilot" ]
