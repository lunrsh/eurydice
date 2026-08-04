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
popd

git clone https://git.ffmpeg.org/ffmpeg.git /tmp/ffmpeg -b n8.1.2
pushd /tmp/ffmpeg
echo $PATH
pushd /tmp/ffmpeg_dist
find . -type f
popd
./configure --arch=x86_64 --target-os=mingw32 --prefix=/tmp/ffmpeg_dist --cross-prefix=x86_64-w64-mingw32- --cc="$CC" --cxx="$CXX" --ld="$LD" --disable-programs --enable-gpl --enable-libmp3lame --enable-nonfree --enable-static --disable-shared
make
make install
popd
