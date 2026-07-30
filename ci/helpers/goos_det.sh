#!/usr/bin/env bash
CLI_OS=$1
TARGET_OS="${CLI_OS:=$FALLBACK_OS}"

echo $TARGET_OS
