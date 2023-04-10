#!/usr/bin/bash

# install go dependencies
go mod tidy || exit 1

# run tests
go test ./... || exit 1

# Build go
go build \
    -ldflags "-s -w -X main.version=${VERSION:-dev}"  \
    -o ./go-myapps-app-opentalk \
    . || exit 1
