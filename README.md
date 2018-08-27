Keysync
-------

[![license](https://img.shields.io/badge/license-apache_2.0-blue.svg?style=flat)](https://raw.githubusercontent.com/square/keysync/master/LICENSE.txt)
[![build](https://travis-ci.org/square/keysync.svg?branch=master)](https://travis-ci.org/square/keysync)
[![report](https://goreportcard.com/badge/github.com/square/keysync)](https://goreportcard.com/report/github.com/square/keysync)

Keysync is a program for accessing secrets in [Keywhiz](https://github.com/square/keywhiz).

It is currently under development, and not yet ready for use.

It is intended as a replacement for the FUSE-based [keywhiz-fs](https://github.com/square/keywhiz-fs).

## Getting Started

### Building

Keysync must be built with Go 1.9+. You can build keysync from source:

```
$ git clone https://github.com/square/keysync
$ cd keysync
$ ./build.sh
```

This will generate a binary called `./bin/keysync`

#### Dependencies

Keysync uses [gvt](https://github.com/FiloSottile/gvt) to manage dependencies. All deps should be added using `gvt fetch` and committed into `vendor` directory.

### Testing

Entire test suite:

```
go test
```

Short, unit tests only:

```
go test -short
```

### Running locally

Keysync requires access to Keywhiz to work properly. Assuming you run Keywhiz locally on default port (4444), you can start keysync with:

```
./keysync --config keysync-config.yaml
```
