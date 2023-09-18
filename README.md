# Deprecated
**As of 9/18/23 this project is now deprecated and no longer maintained.**

Keysync
-------

[![license](https://img.shields.io/badge/license-apache_2.0-blue.svg?style=flat)](https://raw.githubusercontent.com/square/keysync/master/LICENSE.txt)
[![report](https://goreportcard.com/badge/github.com/square/keysync)](https://goreportcard.com/report/github.com/square/keysync)

Keysync is a production-ready program for accessing secrets in [Keywhiz](https://github.com/square/keywhiz).

It is a replacement for the now-deprecated FUSE-based [keywhiz-fs](https://github.com/square/keywhiz-fs).

## Getting Started

### Building

Keysync must be built with Go 1.11+. You can build keysync from source:

```
$ git clone https://github.com/square/keysync
$ cd keysync
$ go build github.com/square/keysync/cmd/keysync
```

This will generate a binary called `./keysync`

#### Dependencies

Keysync uses Go modules to manage dependencies. If you've cloned the repo into `GOPATH`, you should export `GO111MODULE=on` before running any `go` commands. All deps should be automatically fetched when using `go build` and `go test`. Add `go mod tidy` before committing.

### Testing

Entire test suite:

```
go test ./...
```

Short, unit tests only:

```
go test -short ./...
```

### Running locally

Keysync requires access to Keywhiz to work properly. Assuming you run Keywhiz locally on default port (4444), you can start keysync with:

```
./keysync --config keysync-config.yaml
```
