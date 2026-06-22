# tfq build — derives the version string from git and injects it at build time.
#
# VERSION = yyyymmdd.<nth-commit-of-the-day>.<short-hash>   e.g. 20260622.26.e7937bf
#   yyyymmdd               committer date of HEAD
#   nth-commit-of-the-day  commits reachable from HEAD on that calendar day
#   short-hash             `git rev-parse --short HEAD`
# Falls back to "dev" outside a git repo.

VERSION := $(shell \
	d=$$(git show -s --format=%cd --date=format:%Y%m%d HEAD 2>/dev/null) && \
	n=$$(git log --format=%cd --date=format:%Y%m%d 2>/dev/null | grep -c "^$$d") && \
	h=$$(git rev-parse --short HEAD 2>/dev/null) && \
	echo "$$d.$$n.$$h" || echo dev )

LDFLAGS := -X main.version=$(VERSION)

.PHONY: build version test vet clean

build:
	go build -ldflags "$(LDFLAGS)" -o tfq ./cmd/tfq

version:
	@echo $(VERSION)

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f tfq
