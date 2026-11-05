#!/bin/sh
# 卸载前执行：先停服务，避免删掉二进制后进程仍在跑。
# 参数：deb 是 remove / purge / upgrade，rpm 是 0（卸载）或 1（升级），apk 无此参数。
# 注意：本文件必须保持 LF 行尾，CRLF 会让 shebang 变成 "#!/bin/sh\r"，
# dpkg 执行时报 "无法执行 prerm：没有那个文件或目录"，表现为卸载不掉。
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
