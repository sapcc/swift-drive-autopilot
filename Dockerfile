FROM alpine:latest
MAINTAINER "Stefan Majewsky <stefan.majewsky@sap.com>"

RUN apk add --no-cache file
ADD build/docker.tar /
ENTRYPOINT ["/bin/dumb-init", "--", "/bin/swift-drive-autopilot"]
