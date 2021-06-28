FROM golang:1.16-alpine3.13 as builder
WORKDIR /x/src/github.com/sapcc/swift-drive-autopilot/
RUN apk add --no-cache curl make openssl && \
    mkdir -p /pkg/bin/ && \
    curl -L https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64 > /pkg/bin/dumb-init && \
    chmod +x /pkg/bin/dumb-init

COPY . .
ARG VERSION
RUN make install PREFIX=/pkg

################################################################################

FROM alpine:3.13
LABEL source_repository="https://github.com/sapcc/swift-drive-autopilot"

RUN apk add --no-cache file smartmontools
COPY --from=builder /pkg/ /usr/
ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/bin/swift-drive-autopilot"]
