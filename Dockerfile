FROM alpine:latest
MAINTAINER "Stefan Majewsky <stefan.majewsky@sap.com>"

ADD swift-storage-boot /bin/swift-storage-boot
ENTRYPOINT ["/bin/swift-storage-boot"]
