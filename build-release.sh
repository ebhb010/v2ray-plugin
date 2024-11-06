#!/bin/bash
git config --global --add safe.directory "$PWD"

sum="sha1sum"

if ! hash sha1sum 2>/dev/null; then
    if ! hash shasum 2>/dev/null; then
        echo "I can't see 'sha1sum' or 'shasum'"
        echo "Please install one of them!"
        exit
    fi
    sum="shasum"
fi

[[ -z $upx ]] && upx="echo pending"
if [[ $upx == "echo pending" ]] && hash upx 2>/dev/null; then
    upx="upx -9"
fi

VERSION=$(git describe --tags)
LDFLAGS="-X main.VERSION=$VERSION -s -w -buildid="
GCFLAGS=all="-B -C"

OSES=(linux darwin windows freebsd)
ARCHS=(amd64 386)

apt install upx-ucl

wget https://go.dev/dl/go1.22.8.linux-amd64.tar.gz

tar -zxf go1.22.8.linux-amd64.tar.gz
rm go1.22.8.linux-amd64.tar.gz

export PATH=$PWD/go/bin:$PATH

wget https://github.com/eebssk1/aio_tc_build/releases/download/e42fe14d/x86_64-linux-gnu-native.tb2
wget https://github.com/eebssk1/aio_tc_build/releases/download/e42fe14d/x86_64-w64_legacy-mingw32-cross.tb2

tar --bz -xf x86_64-linux-gnu-native.tb2
tar --bz -xf x86_64-w64_legacy-mingw32-cross.tb2
rm *.tb2

export PATH=$PWD/x86_64-linux-gnu/bin:$PWD/x86_64-w64_legacy-mingw32/bin:$PATH


export GOPROXY=direct
export GONOSUMDB=*

mkdir bin

go get

for os in ${OSES[@]}; do
    for arch in ${ARCHS[@]}; do
        # Go 1.15 drops support for 32-bit binaries on macOS, iOS, iPadOS, watchOS, and tvOS (the darwin/386 and darwin/arm ports)
        # Reference URL: https://tip.golang.org/doc/go1.15#darwin
        if [ "$os" == "darwin" ] && [ "$arch" == "386" ]; then
            continue
        fi
        suffix=""
        if [ "$os" == "windows" ]; then
            suffix=".exe"
        fi
        env GOAMD64=v2 CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_${os}_${arch}${suffix}
        $upx v2ray-plugin_${os}_${arch}${suffix} >/dev/null
        tar -zcf bin/v2ray-plugin-${os}-${arch}-$VERSION.tar.gz v2ray-plugin_${os}_${arch}${suffix}
        $sum bin/v2ray-plugin-${os}-${arch}-$VERSION.tar.gz
    done
done

env GOAMD64=v2 CGO_ENABLED=1 CC="x86_64-w64-mingw32-gcc -O3" CGO_LDFLAGS="-Wl,-O3,--relax -static-libgcc -static-libstdc++ -Wl,--push-state,-Bstatic,-lstdc++,-lwinpthread,--pop-state" GOOS=windows GOARCH=amd64 go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_windows_amd64-c.exe
$upx v2ray-plugin_windows_amd64-c.exe >/dev/null
tar -zcf bin/v2ray-plugin-windows-amd64-$VERSION-c.tar.gz v2ray-plugin_windows_amd64-c.exe
$sum bin/v2ray-plugin-windows-amd64-$VERSION-c.tar.gz

env GOAMD64=v2 CGO_ENABLED=1 CC="gcc -O3" CGO_LDFLAGS="-Wl,-O3,--relax -static-libgcc -static-libstdc++" GOOS=linux GOARCH=amd64 go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_linux_amd64-c
$upx v2ray-plugin_linux_amd64-c >/dev/null
tar -zcf bin/v2ray-plugin-linux-amd64-$VERSION-c.tar.gz v2ray-plugin_linux_amd64-c
$sum bin/v2ray-plugin-linux-amd64-$VERSION-c.tar.gz

# ARM
ARMS=(5 6 7)
for v in ${ARMS[@]}; do
    env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=$v go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_linux_arm$v
done
$upx v2ray-plugin_linux_arm* >/dev/null
tar -zcf bin/v2ray-plugin-linux-arm-$VERSION.tar.gz v2ray-plugin_linux_arm*
$sum bin/v2ray-plugin-linux-arm-$VERSION.tar.gz

# ARM64 (ARMv8 or aarch64)
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_linux_arm64
$upx v2ray-plugin_linux_arm64 >/dev/null
tar -zcf bin/v2ray-plugin-linux-arm64-$VERSION.tar.gz v2ray-plugin_linux_arm64
$sum bin/v2ray-plugin-linux-arm64-$VERSION.tar.gz

# Darwin ARM64
env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_darwin_arm64
$upx v2ray-plugin_darwin_arm64 >/dev/null
tar -zcf bin/v2ray-plugin-darwin-arm64-$VERSION.tar.gz v2ray-plugin_darwin_arm64
$sum bin/v2ray-plugin-darwin-arm64-$VERSION.tar.gz

# Windows ARM
env CGO_ENABLED=0 GOOS=windows GOARCH=arm go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_windows_arm.exe
$upx v2ray-plugin_windows_arm.exe >/dev/null
tar -zcf bin/v2ray-plugin-windows-arm-$VERSION.tar.gz v2ray-plugin_windows_arm.exe
$sum bin/v2ray-plugin-windows-arm-$VERSION.tar.gz

# Windows ARM64
env CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_windows_arm64.exe
$upx v2ray-plugin_windows_arm64.exe >/dev/null
tar -zcf bin/v2ray-plugin-windows-arm64-$VERSION.tar.gz v2ray-plugin_windows_arm64.exe
$sum bin/v2ray-plugin-windows-arm64-$VERSION.tar.gz

# MIPS
MIPSS=(mips mipsle)
for v in ${MIPSS[@]}; do
    env CGO_ENABLED=0 GOOS=linux GOARCH=$v go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_linux_$v
    env CGO_ENABLED=0 GOOS=linux GOARCH=$v GOMIPS=softfloat go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_linux_${v}_sf
done
$upx v2ray-plugin_linux_mips* >/dev/null
tar -zcf bin/v2ray-plugin-linux-mips-$VERSION.tar.gz v2ray-plugin_linux_mips*
$sum bin/v2ray-plugin-linux-mips-$VERSION.tar.gz

# MIPS64
MIPS64S=(mips64 mips64le)
for v in ${MIPS64S[@]}; do
    env CGO_ENABLED=0 GOOS=linux GOARCH=$v go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_linux_$v
done
tar -zcf bin/v2ray-plugin-linux-mips64-$VERSION.tar.gz v2ray-plugin_linux_mips64*
$sum bin/v2ray-plugin-linux-mips64-$VERSION.tar.gz

# ppc64le
env CGO_ENABLED=0 GOOS=linux GOARCH=ppc64le go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_linux_ppc64le
$upx v2ray-plugin_linux_ppc64le >/dev/null
tar -zcf bin/v2ray-plugin-linux-ppc64le-$VERSION.tar.gz v2ray-plugin_linux_ppc64le
$sum bin/v2ray-plugin-linux-ppc64le-$VERSION.tar.gz

# s390x
env CGO_ENABLED=0 GOOS=linux GOARCH=s390x go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_linux_s390x
$upx v2ray-plugin_linux_s390x >/dev/null
tar -zcf bin/v2ray-plugin-linux-s390x-$VERSION.tar.gz v2ray-plugin_linux_s390x
$sum bin/v2ray-plugin-linux-s390x-$VERSION.tar.gz

# riscv64
env CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -v -trimpath -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o v2ray-plugin_linux_riscv64
$upx v2ray-plugin_linux_riscv64 >/dev/null
tar -zcf bin/v2ray-plugin-linux-riscv64-$VERSION.tar.gz v2ray-plugin_linux_riscv64
$sum bin/v2ray-plugin-linux-riscv64-$VERSION.tar.gz
