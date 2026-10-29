#!/bin/sh
# apt / yum / apk 安装或升级后执行。
# 参数：deb 是 configure / abort-*，rpm 是 1（安装）或 2（升级），apk 无此参数。
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi

case "$1" in
    configure|1|2|"")
        echo "fast-https: 配置目录 /usr/share/fast-https/config，日志目录 /usr/share/fast-https/logs"
        echo "fast-https: 启动命令 systemctl enable --now fast-https"
        echo "fast-https: 这些目录归 root 所有，手工执行 start/check 请用 sudo"
        ;;
esac

exit 0
