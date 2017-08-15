FROM golang:1.8.3-alpine3.6 as builder
WORKDIR /x/src/github.com/sapcc/swift-drive-autopilot/
RUN apk add --no-cache curl make openssl && \
    mkdir -p /pkg/bin/ && \
    curl https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64 > /pkg/bin/dumb-init && \
    chmod +x /pkg/bin/dumb-init

COPY . .
ARG VERSION
RUN make install DESTDIR=/pkg/

################################################################################

FROM alpine:latest
MAINTAINER "Stefan Majewsky <stefan.majewsky@sap.com>"
RUN apk add --no-cache file

COPY --from=builder /pkg/ /usr/
ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/bin/swift-drive-autopilot"]
