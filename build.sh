#!/bin/bash
set -eux

# This build.sh assumes you've cloned this repository somewhere, and don't have
# a proper $GOPATH.  Thus it makes a temporary directory so the go tooling can
# work out of the box.  Hopefully this use case is handled "out of the box" by
# the go toolchain in the future, and so we can remove this.

# TODO: Make this more configurable:
#       - Where the output binary goes, to keep the srcdir clean
#       - make sure it works on many systems (tested on mac os and centos7)
#       - rm -r is scary.


# Temporary directory to work in
TMPWORK=$(mktemp -d)
# Need an absolute path for `go`
GOPATH=$(realpath "$TMPWORK")
export GOPATH

SRCDIR=$(realpath "$(dirname "${BASH_SOURCE[0]}")")

mkdir -p "$GOPATH/src/github.com/square/"
ln -s "$SRCDIR" "$GOPATH/src/github.com/square/keysync"

cd "$GOPATH/src/github.com/square/keysync" || exit 1

# Check that dep is installed.
if ! [ -x "$(command -v dep)" ]; then
  echo 'dep dependency manager is missing. Installing now.'
  go get -d -u github.com/golang/dep
fi

# Check if dependencies are up-to-date and update them otherwise.
if [ "$(dep check -q)" != 0 ]; then
  dep ensure -vendor-only
fi

LDFLAGS="-X main.release=$(git show --format=%H --no-patch)"
go build -ldflags "$LDFLAGS" -o "${SRCDIR}/keysync" ./cmd/keysync
go build -o "${SRCDIR}/keyrestore" ./cmd/keyrestore

rm -rf "$TMPWORK"
