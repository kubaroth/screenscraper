#!/usr/bin/bash
archs=(amd64 arm64 arm)

for arch in ${archs[@]}
do
    env GOOS=linux GOARCH=${arch} go build -o screenscraper_${arch} screenscraper.go 
done
