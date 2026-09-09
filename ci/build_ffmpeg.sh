#!/usr/bin/env bash
set -xeuo pipefail

export CC="zig cc -target x86_64-windows-gnu"
export CXX="zig c++ -target x86_64-windows-gnu"
export LD="zig cc -target x86_64-windows-gnu"
export AR="zig ar"

mkdir -p /tmp/ffmpeg /tmp/libmp3lame /tmp/ffmpeg_dist

pushd /tmp/libmp3lame
curl -L https://sourceforge.net/projects/lame/files/lame/3.100/lame-3.100.tar.gz > lame.tar.gz
tar -xzvf lame.tar.gz
cd lame-3.100
./configure --host=x86_64-w64-mingw32 --prefix=/tmp/ffmpeg_dist --disable-shared --enable-static --disable-frontend --disable-dependency-tracking --disable-decoder
sed -i \
  's|old_archive_cmds="lib -OUT:\\$oldlib\\$oldobjs\\$old_deplibs"|old_archive_cmds="\\$AR rcs \\$oldlib \\$oldobjs"|' \
  libtool
make -j$(nproc)
make install
mv /tmp/ffmpeg_dist/lib/libmp3lame.la /tmp/ffmpeg_dist/lib/mp3lame.la # ffmpeg expects a diff path, so we meet its expectation
mv /tmp/ffmpeg_dist/lib/libmp3lame.lib /tmp/ffmpeg_dist/lib/mp3lame.lib
popd

git clone https://git.ffmpeg.org/ffmpeg.git /tmp/ffmpeg -b n8.1.2
pushd /tmp/ffmpeg
# from https://github.com/mcmtroffaes/ffmpeg-msvc-build/issues/5
set +euo pipefail

./configure \
    --arch=x86_64 \
    --target-os=mingw32 \
    --prefix=/tmp/ffmpeg_dist \
    --cross-prefix=x86_64-w64-mingw32- \
    --cc="$CC" \
    --cxx="$CXX" \
    --ld="$LD" \
    --ar="$AR" \
    --extra-cflags="-I/tmp/ffmpeg_dist/include" \
    --extra-ldflags="-L/tmp/ffmpeg_dist/lib" \
    --extra-libs="-lm" \
	--disable-shared \
	--enable-static \
	--disable-debug \
	--disable-programs \
	--enable-gpl \
	--enable-nonfree \
	--disable-doc \
	--disable-avdevice \
	--disable-swscale \
	--disable-iconv \
	--disable-zlib \
	--disable-bzlib \
	--disable-lzma \
	--disable-sdl2 \
	--disable-schannel \
	--disable-securetransport \
	--disable-xlib \
	--disable-muxers \
	--disable-demuxers \
	--disable-hwaccels \
	--disable-d3d11va \
	--disable-nvenc \
	--disable-dxva2 \
	--disable-bsfs \
	--disable-filters \
	--disable-parsers \
	--disable-indevs \
	--disable-outdevs \
	--disable-encoders \
	--disable-decoders \
	--enable-libmp3lame \
	--enable-encoder=libmp3lame \
	--enable-encoder=flac \
	--enable-encoder=alac \
	--enable-encoder=aac \
	--enable-muxer=flac \
	--enable-muxer=caf \
	--enable-demuxer=caf \
	--enable-muxer=aiff \
	--enable-muxer=mp3 \
	--enable-demuxer=aac \
	--enable-demuxer=ac3 \
	--enable-demuxer=aiff \
	--enable-demuxer=ape \
	--enable-demuxer=asf \
	--enable-demuxer=au \
	--enable-demuxer=avi \
	--enable-demuxer=flac \
	--enable-demuxer=flv \
	--enable-demuxer=matroska \
	--enable-demuxer=mov \
	--enable-demuxer=m4v \
	--enable-demuxer=mp3 \
	--enable-demuxer=mpc* \
	--enable-demuxer=ogg \
	--enable-demuxer=pcm* \
	--enable-demuxer=rm \
	--enable-demuxer=shorten \
	--enable-demuxer=tak \
	--enable-demuxer=tta \
	--enable-demuxer=wav \
	--enable-demuxer=wv \
	--enable-demuxer=xwma \
	--enable-demuxer=dsf \
	--enable-demuxer=dts \
	--enable-demuxer=truehd \
	--enable-decoder=aac* \
	--enable-decoder=ac3 \
	--enable-decoder=alac \
	--enable-decoder=als \
	--enable-decoder=ape \
	--enable-decoder=atrac* \
	--enable-decoder=eac3 \
	--enable-decoder=flac \
	--enable-decoder=gsm* \
	--enable-decoder=mp1* \
	--enable-decoder=mp2* \
	--enable-decoder=mp3* \
	--enable-decoder=mpc* \
	--enable-decoder=opus \
	--enable-decoder=ra* \
	--enable-decoder=ralf \
	--enable-decoder=shorten \
	--enable-decoder=tak \
	--enable-decoder=tta \
	--enable-decoder=vorbis \
	--enable-decoder=wavpack \
	--enable-decoder=wma* \
	--enable-decoder=pcm* \
	--enable-decoder=dsd* \
	--enable-decoder=truehd \
	--enable-parser=aac* \
	--enable-parser=ac3 \
	--enable-parser=cook \
	--enable-parser=dca \
	--enable-parser=flac \
	--enable-parser=gsm \
	--enable-parser=mpegaudio \
	--enable-parser=tak \
	--enable-parser=vorbis

HAS_FAILED=$?

cat ffbuild/config.log

if [ $HAS_FAILED -ne 0 ]; then
    echo "Configuration failed!!"
    exit 1
fi

set -euo pipefail

make
make install
popd

set +x
pushd /tmp/ffmpeg_dist
echo "Done! ffmpeg-related files:"
find . -type f
popd
