all: swift-drive-autopilot

swift-drive-autopilot: *.go
	go build -ldflags '-s -w' -o $@ .
