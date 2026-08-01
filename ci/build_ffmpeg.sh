#!/usr/bin/env bash
set -xeuo pipefail

export CC="zig cc -target x86_64-windows-gnu"
export CXX="zig c++ -target x86_64-windows-gnu"
export LD="zig cc -target x86_64-windows-gnu"
export AR="zig ar"

mkdir -p /tmp/ffmpeg /tmp/libmp3lame
pushd /tmp/libmp3lame
curl -L https://sourceforge.net/projects/lame/files/lame/3.100/lame-3.100.tar.gz > lame.tar.gz
tar -xzvf lame.tar.gz
cd lame-3.100
./configure --host=x86_64-w64-mingw32 --disable-shared --enable-static --cc="$CC" --cxx="$CXX" --ld="$LD"
make -j$(nproc)
sudo make install # TODO: unsafe, migrate to prefix soon
popd

git clone https://git.ffmpeg.org/ffmpeg.git /tmp/ffmpeg -b n8.1.2
pushd /tmp/ffmpeg
./configure --arch=x86_64 --target-os=mingw32 --cross-prefix=x86_64-w64-mingw32- --cc="$CC" --cxx="$CXX" --ld="$LD" --disable-programs --enable-gpl --enable-libmp3lame --enable-nonfree --enable-static --disable-shared
make
sudo make install # TODO: unsafe, migrate to prefix soon
popd
