all: swift-storage-boot

swift-storage-boot: *.go
	go build -ldflags '-s -w' -o $@ .
