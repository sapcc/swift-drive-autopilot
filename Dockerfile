FROM golang:1.8.3-alpine3.6 as builder
WORKDIR /x/src/github.com/sapcc/swift-drive-autopilot/
COPY . .
RUN apk add --no-cache make && \
    mkdir -p /pkg/bin/ && \
    wget -O /pkg/bin/dumb-init https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64 && \
    chmod +x /pkg/bin/dumb-init

ARG VERSION
RUN make install DESTDIR=/pkg/

################################################################################

FROM alpine:latest
MAINTAINER "Stefan Majewsky <stefan.majewsky@sap.com>"
RUN apk add --no-cache file

COPY --from=builder /pkg/ /usr/
ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/bin/swift-drive-autopilot"]
