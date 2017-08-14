PKG    = github.com/sapcc/swift-drive-autopilot
PREFIX := /usr

all: build/swift-drive-autopilot

# force people to use golangvend
GO            := GOPATH=$(CURDIR)/.gopath GOBIN=$(CURDIR)/build go
GO_BUILDFLAGS :=
GO_LDFLAGS    := -s -w

# This target uses the incremental rebuild capabilities of the Go compiler to speed things up.
# If no source files have changed, `go install` exits quickly without doing anything.
build/swift-drive-autopilot: FORCE
	$(GO) install $(GO_BUILDFLAGS) -ldflags '$(GO_LDFLAGS)' '$(PKG)'
build/logexpect: FORCE
	$(GO) install $(GO_BUILDFLAGS) -ldflags '$(GO_LDFLAGS)' '$(PKG)/cmd/logexpect'

test: FORCE all build/logexpect
	$(GO) test '$(PKG)/cmd/logexpect'
	./test/run.sh
check: test # just a synonym

install: FORCE all
	install -D -m 0755 build/swift-drive-autopilot "$(DESTDIR)$(PREFIX)/bin/swift-drive-autopilot"

vendor:
	@# vendoring by https://github.com/holocm/golangvend
	@golangvend

.PHONY: vendor test check FORCE
