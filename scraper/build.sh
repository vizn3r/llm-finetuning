#!/bin/bash
echo "Deleting old builds"
rm -rf ./build/*

go build -o ./build/scrape-wiki wiki.go
echo "Building 'scrape-wiki'"

echo "Done"
