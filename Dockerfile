FROM alpine:latest
MAINTAINER "Stefan Majewsky <stefan.majewsky@sap.com>"

# file(1) is required for the autopilot, wget(1) is only used to retrieve dumb-init
RUN apk update && \
    apk add file wget ca-certificates && \
    wget -O /bin/dumb-init https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64 && \
    chmod +x /bin/dumb-init && \
    apk del wget ca-certificates

ADD swift-drive-autopilot /bin/swift-drive-autopilot
ENTRYPOINT ["/bin/dumb-init", "--", "/bin/swift-drive-autopilot"]
