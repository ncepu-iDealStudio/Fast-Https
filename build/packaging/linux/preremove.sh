#!/bin/sh
# 卸载前执行：先停服务，避免删掉二进制后进程仍在跑。
# 参数：deb 是 remove / purge / upgrade，rpm 是 0（卸载）或 1（升级），apk 无此参数。
set -e

case "$1" in
    remove|purge|0|"")
        if command -v systemctl >/dev/null 2>&1; then
            systemctl stop fast-https >/dev/null 2>&1 || true
            systemctl disable fast-https >/dev/null 2>&1 || true
        fi
        ;;
esac

exit 0
