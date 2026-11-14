#!/bin/sh
# apt / yum / apk 安装或升级后执行。
# 参数：deb 是 configure / abort-*，rpm 是 1（安装）或 2（升级），apk 无此参数。
# 注意：本文件必须保持 LF 行尾，CRLF 会让 shebang 变成 "#!/bin/sh\r"，
# dpkg/rpm 执行时报 ENOENT（tests/packaging 会检查）。
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi

# FHS 布局下的可写目录。systemd 的 LogsDirectory/RuntimeDirectory 会建，
# 但 Alpine 没有 systemd，手工启动（非单元托管）时也得先存在，否则写日志失败。
mkdir -p /etc/fast-https/cert/acme /etc/fast-https/conf.d /var/log/fast-https /run/fast-https /var/lib/fast-https /var/lib/fast-https/agent
chmod 0700 /etc/fast-https/cert /etc/fast-https/cert/acme /var/lib/fast-https 2>/dev/null || true

# 早期版本的包把所有运行资源放在 /usr/share/fast-https/ 下。
# 升级到 FHS 布局后配置与证书在 /etc/fast-https，这里做一次迁移：
# 只复制不删除、目标已存在就跳过，失败也不中断安装（旧文件仍留在原处）。
LEGACY_DIR=/usr/share/fast-https/config
ETC_DIR=/etc/fast-https
if [ -d "$LEGACY_DIR" ]; then
    ( cd "$LEGACY_DIR" && find . -type f -print ) | while IFS= read -r rel; do
        rel="${rel#./}"
        src="$LEGACY_DIR/$rel"
        dst="$ETC_DIR/$rel"
        if [ -e "$dst" ]; then
            continue
        fi
        if ! mkdir -p "$(dirname "$dst")" 2>/dev/null; then
            echo "fast-https: cannot create $(dirname "$dst"), keep $src" >&2
            continue
        fi
        if ! cp -p "$src" "$dst" 2>/dev/null; then
            echo "fast-https: migrate $rel failed, keep $src" >&2
        fi
    done
    echo "fast-https: migrated legacy config from $LEGACY_DIR (old files kept, remove them manually after checking)"
fi

case "$1" in
    configure|1|2|"")
        echo "fast-https: config /etc/fast-https, site /usr/share/fast-https/httpdoc, log /var/log/fast-https"
        echo "fast-https: start with systemctl enable --now fast-https"
        echo "fast-https: these directories are owned by root, run start/check with sudo"
        ;;
esac

exit 0
