#!/bin/bash -xe

if [ -z $RELEASE_VERSION ];
then
  echo "Release version (export RELEASE_VERSION) must be specified, example: zbchaos-v0.1.0"
  exit 1
fi

OS=( linux windows darwin )
BINARY=( zbchaos zbchaos.exe zbchaos.darwin )
SRC_DIR=$(dirname "${BASH_SOURCE[0]}")
DIST_DIR="$SRC_DIR/dist"

VERSION=${RELEASE_VERSION:-development}
COMMIT=${RELEASE_HASH:-$(git rev-parse HEAD)}

mkdir -p ${DIST_DIR}
rm -rf ${DIST_DIR}/*

for i in "${!OS[@]}"; do
	if [ $# -eq 0 ] || [ ${OS[$i]} = $1 ]; then
	    CGO_ENABLED=0 GOOS="${OS[$i]}" GOARCH=amd64 go build -a -tags netgo -ldflags "-w -X github.com/camunda/zeebe-chaos/go-chaos/cmd.Version=${VERSION} -X github.com/camunda/zeebe-chaos/go-chaos/cmd.Commit=${COMMIT}" -o "${DIST_DIR}/${BINARY[$i]}" "${SRC_DIR}/main.go" # &
	fi
done

# wait
