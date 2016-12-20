all: swift-drive-autopilot

# force people to use golangvend
GOCC := env GOPATH=$(CURDIR)/.gopath go
GOFLAGS := -ldflags '-s -w'

swift-drive-autopilot: *.go
	$(GOCC) build $(GOFLAGS) -o $@ github.com/sapcc/swift-drive-autopilot

vendor:
	@golangvend
.PHONY: vendor
