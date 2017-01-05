all: swift-drive-autopilot

# force people to use golangvend
GOCC := env GOPATH=$(CURDIR)/.gopath go
GOFLAGS := -ldflags '-s -w'

swift-drive-autopilot: *.go
	$(GOCC) build $(GOFLAGS) -o $@ github.com/sapcc/swift-drive-autopilot

test: swift-drive-autopilot test/logexpect
	./test/run.sh
check: test # just a synonym

test/logexpect: cmd/logexpect/*.go
	$(GOCC) build $(GOFLAGS) -o $@ github.com/sapcc/swift-drive-autopilot/cmd/logexpect
	$(GOCC) test github.com/sapcc/swift-drive-autopilot/cmd/logexpect

vendor:
	@golangvend

.PHONY: vendor test check
