# renovate: datasource=docker depName=alpine versioning=docker
ARG ALPINE_VERSION=3.15
# renovate: datasource=docker depName=golang versioning=docker
ARG GOLANG_VERSION=1.18.2-alpine

FROM golang:${GOLANG_VERSION}${ALPINE_VERSION} as builder
WORKDIR /x/src/github.com/sapcc/swift-drive-autopilot/
RUN apk add --no-cache curl make openssl && \
    mkdir -p /pkg/bin/ && \
    curl -L https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64 > /pkg/bin/dumb-init && \
    chmod +x /pkg/bin/dumb-init

COPY . .
ARG VERSION
RUN make install PREFIX=/pkg

################################################################################

FROM alpine:${ALPINE_VERSION}
LABEL source_repository="https://github.com/sapcc/swift-drive-autopilot"

RUN apk add --no-cache file smartmontools
COPY --from=builder /pkg/ /usr/
ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/bin/swift-drive-autopilot"]
