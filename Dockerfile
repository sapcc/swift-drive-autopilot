FROM golang:1.20.1-alpine3.17 as builder

RUN apk add --no-cache --no-progress gcc git make musl-dev

COPY . /src
ARG BININFO_BUILD_DATE BININFO_COMMIT_HASH BININFO_VERSION # provided to 'make install'
RUN make -C /src install PREFIX=/pkg GO_BUILDFLAGS='-mod vendor'

################################################################################

FROM alpine:3.17

RUN addgroup -g 4200 appgroup
RUN adduser -h /home/appuser -s /sbin/nologin -G appgroup -D -u 4200 appuser
RUN apk add --no-cache --no-progress ca-certificates dumb-init file smartmontools
COPY --from=builder /pkg/ /usr/

ARG BININFO_BUILD_DATE BININFO_COMMIT_HASH BININFO_VERSION
LABEL source_repository="https://github.com/sapcc/swift-drive-autopilot" \
  org.opencontainers.image.url="https://github.com/sapcc/swift-drive-autopilot" \
  org.opencontainers.image.created=${BININFO_BUILD_DATE} \
  org.opencontainers.image.revision=${BININFO_COMMIT_HASH} \
  org.opencontainers.image.version=${BININFO_VERSION}

WORKDIR /var/empty
ENTRYPOINT [ "/usr/bin/dumb-init", "--", "/usr/bin/swift-drive-autopilot" ]
