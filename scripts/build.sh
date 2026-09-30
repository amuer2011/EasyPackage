#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
output_root=${EASYPACKAGE_DIST:-dist/v1.0.0}
host_os=$(go env GOOS)
host_arch=$(go env GOARCH)

build() {
    platform=$1
    architecture=$2
    destination="$output_root/$platform-$architecture"
    mkdir -p "$destination"
    filename=EasyPackage
    flags='-s -w'
    cgo=0
    if [ "$platform" = windows ]; then
        filename=EasyPackage.exe
        flags='-s -w -H=windowsgui'
    elif [ "$platform" = "$host_os" ]; then
        cgo=1
    else
        echo "Native GUI targets must be built on their own OS (requested $platform on $host_os)." >&2
        exit 2
    fi
    CGO_ENABLED=$cgo GOOS="$platform" GOARCH="$architecture" go build -trimpath -ldflags "$flags" -o "$destination/$filename" .
    cp README.md "$destination/README.md"
    printf 'Ready: %s/%s\n' "$destination" "$filename"
}

case "${1:-}" in
    --all)
        build windows amd64
        build windows arm64
        case "$host_os" in
            darwin|linux) build "$host_os" "$host_arch" ;;
            *) ;;
        esac
        ;;
    --windows)
        build windows amd64
        build windows arm64
        ;;
    --native)
        case "$host_os" in
            darwin|linux) build "$host_os" "$host_arch" ;;
            *) echo "Native macOS/Linux builds must run on macOS or Linux." >&2; exit 2 ;;
        esac
        ;;
    "")
        case "$host_os" in
            darwin|linux) build "$host_os" "$host_arch" ;;
            windows) build windows "$host_arch" ;;
            *) echo "Unsupported host OS: $host_os" >&2; exit 2 ;;
        esac
        ;;
    *)
        echo "Usage: $0 [--native|--windows|--all]" >&2
        exit 2
        ;;
esac
