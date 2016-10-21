FROM alpine:latest
MAINTAINER "Stefan Majewsky <stefan.majewsky@sap.com>"

RUN apk update && apk add file

ADD swift-drive-autopilot /bin/swift-drive-autopilot
ENTRYPOINT ["/bin/swift-drive-autopilot"]
