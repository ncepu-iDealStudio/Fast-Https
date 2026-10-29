#!/bin/sh
# 卸载或升级后执行：让 systemd 重新读取单元目录。
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi

exit 0
