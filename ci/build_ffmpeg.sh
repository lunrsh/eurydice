#!/usr/bin/env bash
set -xeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

CLI_OS="${1:-}"
CLI_ARCH="${2:-}"

TARGET_OS="$(FALLBACK_OS="${FALLBACK_OS:-$(uname -s | tr '[:upper:]' '[:lower:]')}" "$REPO_ROOT/ci/helpers/goos_det.sh" "$CLI_OS")"
TARGET_ARCH="$(FALLBACK_ARCH="${FALLBACK_ARCH:-$(uname -m)}" "$REPO_ROOT/ci/helpers/goarch_det.sh" "$CLI_OS" "$CLI_ARCH")"

# Normalize uname -m values if FALLBACK_ARCH was used directly
if [ "$TARGET_ARCH" = "x86_64" ]; then
  TARGET_ARCH="amd64"
elif [ "$TARGET_ARCH" = "aarch64" ]; then
  TARGET_ARCH="arm64"
fi

case "$TARGET_ARCH" in
  amd64)
    FFMPEG_ARCH="x86_64"
    HOST_ARCH="x86_64"
    ;;
  arm64)
    FFMPEG_ARCH="aarch64"
    HOST_ARCH="aarch64"
    ;;
  *)
    echo "Unsupported architecture: $TARGET_ARCH" >&2
    exit 1
    ;;
esac

case "$TARGET_OS" in
  windows)
    ZIG_TARGET="${HOST_ARCH}-windows-gnu"
    LAME_HOST="${HOST_ARCH}-w64-mingw32"
    FFMPEG_OS="mingw32"
    CC_CMD="zig cc -target $ZIG_TARGET"
    CXX_CMD="zig c++ -target $ZIG_TARGET"
    ;;
  linux)
    ZIG_TARGET="${HOST_ARCH}-linux-musl"
    LAME_HOST="${HOST_ARCH}-linux-musl"
    FFMPEG_OS="linux"
    CC_CMD="zig cc -target $ZIG_TARGET"
    CXX_CMD="zig c++ -target $ZIG_TARGET"
    ;;
  darwin)
    LAME_HOST="${HOST_ARCH}-apple-darwin"
    FFMPEG_OS="darwin"
    if [ "$(uname -s)" = "Darwin" ]; then
      # On macOS, omit -target so zig cc uses native Darwin SDK detection
      CC_CMD="zig cc"
      CXX_CMD="zig c++"
    else
      ZIG_TARGET="${HOST_ARCH}-macos-none"
      CC_CMD="zig cc -target $ZIG_TARGET"
      CXX_CMD="zig c++ -target $ZIG_TARGET"
    fi
    ;;
  *)
    echo "Unsupported OS: $TARGET_OS" >&2
    exit 1
    ;;
esac

export CC="$CC_CMD"
export CXX="$CXX_CMD"
export LD="$CC_CMD"
export AR="zig ar"
export RANLIB="zig ranlib"

FFMPEG_DIST="$REPO_ROOT/dist/ffmpeg_dist/${TARGET_OS}_${TARGET_ARCH}"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/eurydice-ffmpeg-build.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT

mkdir -p "$WORK_DIR/libmp3lame" "$FFMPEG_DIST"

if command -v nproc >/dev/null 2>&1; then
  JOBS="$(nproc)"
elif command -v sysctl >/dev/null 2>&1; then
  JOBS="$(sysctl -n hw.ncpu)"
else
  JOBS=4
fi

pushd "$WORK_DIR/libmp3lame"
curl -fL --retry 3 https://tarballs.nixos.org/sha256/07nsn5sy3a8xbmw1bidxnsj5fj6kg9ai04icmqw40ybkp353dznx -o lame.tar.gz || \
  curl -fL --retry 3 https://downloads.sourceforge.net/project/lame/lame/3.100/lame-3.100.tar.gz -o lame.tar.gz
tar -xzvf lame.tar.gz
cd lame-3.100
./configure \
  --host="$LAME_HOST" \
  --prefix="$FFMPEG_DIST" \
  --disable-shared \
  --enable-static \
  --disable-frontend \
  --disable-dependency-tracking \
  --disable-decoder

if [ "$TARGET_OS" = "windows" ]; then
  sed -i.bak \
    's|old_archive_cmds="lib -OUT:\\$oldlib\\$oldobjs\\$old_deplibs"|old_archive_cmds="\\$AR rcs \\$oldlib \\$oldobjs"|' \
    libtool
fi

make -j"$JOBS"
make install

if [ "$TARGET_OS" = "windows" ] && [ -f "$FFMPEG_DIST/lib/libmp3lame.lib" ]; then
  cp -f "$FFMPEG_DIST/lib/libmp3lame.la" "$FFMPEG_DIST/lib/mp3lame.la"
  cp -f "$FFMPEG_DIST/lib/libmp3lame.lib" "$FFMPEG_DIST/lib/mp3lame.lib"
  cp -f "$FFMPEG_DIST/lib/libmp3lame.lib" "$FFMPEG_DIST/lib/libmp3lame.a"
fi
popd

git clone --depth 1 https://git.ffmpeg.org/ffmpeg.git "$WORK_DIR/ffmpeg" -b n8.1.2
pushd "$WORK_DIR/ffmpeg"

# zig cc's Darwin linker wrapper rejects -Wl,-dynamic, so we check the flag instead of adding it unconditionally
sed -i.bak 's/add_ldflags -Wl,-dynamic,-search_paths_first/check_ldflags -Wl,-dynamic,-search_paths_first/' configure

# Extract all built-in audio, PCM, DPCM, and ADPCM encoders and decoders, plus audio filters
AUDIO_ENCODERS="$(awk '/\/\* audio codecs \*\//,/\/\* subtitles \*\//' libavcodec/allcodecs.c | sed -n 's/^extern const FFCodec ff_\(.*\)_encoder;/\1/p' | paste -sd, -)"
AUDIO_DECODERS="$(awk '/\/\* audio codecs \*\//,/\/\* subtitles \*\//' libavcodec/allcodecs.c | sed -n 's/^extern const FFCodec ff_\(.*\)_decoder;/\1/p' | paste -sd, -)"
AUDIO_FILTERS="$(sed -n 's/^extern const FFFilter ff_a\(f\|src\|sink\)_\(.*\);/\2/p' libavfilter/allfilters.c | paste -sd, -)"

EXTRA_CONFIGURE_FLAGS=()
if ! command -v nasm >/dev/null 2>&1 && ! command -v yasm >/dev/null 2>&1; then
  if [ "$TARGET_ARCH" = "amd64" ]; then
    EXTRA_CONFIGURE_FLAGS+=(--disable-x86asm)
  fi
fi

./configure \
  --arch="$FFMPEG_ARCH" \
  --target-os="$FFMPEG_OS" \
  --enable-cross-compile \
  --prefix="$FFMPEG_DIST" \
  --cc="$CC" \
  --cxx="$CXX" \
  --ld="$LD" \
  --host-cc="zig cc" \
  --host-ld="zig cc" \
  --ar="$AR" \
  --ranlib="$RANLIB" \
  --strip="strip" \
  --extra-cflags="-I$FFMPEG_DIST/include" \
  --extra-ldflags="-L$FFMPEG_DIST/lib" \
  --disable-programs \
  --disable-doc \
  --disable-network \
  --disable-autodetect \
  --disable-hwaccels \
  --disable-devices \
  --disable-encoders \
  --disable-decoders \
  --disable-filters \
  --enable-filter="abuffer,abuffersink,$AUDIO_FILTERS" \
  --enable-encoder="libmp3lame,$AUDIO_ENCODERS" \
  --enable-decoder="$AUDIO_DECODERS" \
  --enable-gpl \
  --enable-nonfree \
  --enable-libmp3lame \
  --enable-static \
  --disable-shared \
  "${EXTRA_CONFIGURE_FLAGS[@]}"

make -j"$JOBS"
make install
popd

set +x
pushd "$FFMPEG_DIST"
echo "Done! ffmpeg-related files in $FFMPEG_DIST:"
find . -type f
popd
