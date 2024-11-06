#!/bin/bash

GO_BUILD_DIR=../build

echo "Deleting old builds"
rm -rf "${GO_BUILD_DIR}/*"

go build -o "${GO_BUILD_DIR}/scrape-wiki" wiki.go
echo "Building 'scrape-wiki'"

echo "Done"
