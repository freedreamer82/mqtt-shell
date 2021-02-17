#!/bin/bash

rm -rf dvc-evo-go*
 env GOOS=linux GOARCH=arm64 go build  -o mqtt-shell-arm64 -ldflags '-w -s'
 env GOOS=linux GOARCH=arm go build -o  mqtt-shell-arm32 -ldflags '-w -s '
 env GOOS=linux GOARCH=amd64 go build -o  mqtt-shell-x86-64 -ldflags '-w -s '
 env GOOS=linux GOARCH=386  go build -o  mqtt-shell-x86-32 -ldflags '-w -s'


