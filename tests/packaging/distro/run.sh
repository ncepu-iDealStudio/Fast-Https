#!/bin/sh
# 在容器里跑发行版实机验证。
#
#   sh tests/packaging/distro/run.sh all          # deb → upgrade → rpm → apk → desktop
#   sh tests/packaging/distro/run.sh deb arm64    # 指定架构
#
# 前置：本机有 docker，且 dist/ 下已经有包
#   goreleaser release -f build/goreleaser.yaml --snapshot --clean --skip=archive,publish,announce
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
action=${1:-all}
arch=${2:-amd64}

# Git Bash / MSYS 会把 -v 里的 /dist 改写成 Windows 路径，关掉自动转换。
MSYS_NO_PATHCONV=1
export MSYS_NO_PATHCONV

host_path() {
    if command -v cygpath >/dev/null 2>&1; then
        cygpath -w "$1"
    else
        printf '%s' "$1"
    fi
}

dist=$(host_path "$root/dist")
scripts=$(host_path "$here")

run_one() {
    case "$1" in
        deb)     image=ubuntu:24.04;    script=deb.sh ;;
        rpm)     image=almalinux:9;     script=rpm.sh ;;
        apk)     image=alpine:3.21;     script=apk.sh ;;
        upgrade) image=ubuntu:24.04;    script=upgrade.sh ;;
        desktop) image=ubuntu:24.04;    script=desktop-deb.sh ;;
        *) echo "未知目标: $1（可选 deb / rpm / apk / upgrade / desktop / all）" >&2; exit 2 ;;
    esac

    echo "=== $1（$image, $arch） ==="
    docker run --rm --stop-timeout 5 \
        -e ARCH="$arch" \
        -v "$dist:/dist:ro" \
        -v "$scripts:/scripts:ro" \
        "$image" sh "/scripts/$script" || return $?
}

case "$action" in
    all)
        for target in deb upgrade rpm apk desktop; do
            run_one "$target" || exit $?
        done
        ;;
    *)
        run_one "$action"
        ;;
esac
