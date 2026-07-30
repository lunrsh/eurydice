#!/usr/bin/env bash
set -x
CLI_OS=$1
TARGET_OS="${CLI_OS:=$FALLBACK_OS}"

CLI_ARCH=$2
TARGET_ARCH="${CLI_ARCH:=$FALLBACK_ARCH}"

ASSEMBLED_TARGET_STRING=""

if [ "$TARGET_ARCH" = "arm64" ]; then
    ASSEMBLED_TARGET_STRING+="aarch64-"
elif [ "$TARGET_ARCH" = "amd64" ]; then
    ASSEMBLED_TARGET_STRING+="x86_64-"
else
    exit 1
fi

if [ "$TARGET_OS" = "windows" ]; then
    ASSEMBLED_TARGET_STRING+="windows-gnu"
elif [ "$TARGET_OS" = "darwin" ]; then
    ASSEMBLED_TARGET_STRING+="macos-none"
elif [ "$TARGET_OS" = "linux" ]; then
    ASSEMBLED_TARGET_STRING+="linux-musl"
else
    exit 1
fi

echo $ASSEMBLED_TARGET_STRING
