#!/bin/sh
# 卸载或升级后执行：让 systemd 重新读取单元目录，并在用户明确"不要了"时清数据。
# 参数：deb 是 remove / purge / upgrade；rpm 是 0（卸载）或 1（升级）；apk 无此参数。
# 注意：本文件必须保持 LF 行尾，CRLF 会让 dpkg/rpm 执行时报 ENOENT。
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi

# 三档语义：
#   apt remove / dnf remove  → 保留配置与数据，重装后能接着用
#   apt purge  / rpm -e      → 用户明确不要了，连证书、日志一起清
#   升级（upgrade / $1=1）    → 什么都不动
# apk 的 post-deinstall 拿不到参数，无法区分升级与卸载，一律保留数据。
#
# 删除范围只限本程序生成的东西：
#   /etc/fast-https/cert       CA 私钥、站点证书、ACME 账户与缓存
#   /etc/fast-https/conf.d     站点片段（default.json 是 conffile，dpkg 已删）
#   /var/log/fast-https        四路日志
#   /run/fast-https            pid
#   httpdoc/root/root.crt      本地 CA 证书，私钥在 cert/ 里，留着会成为孤儿
# 站点根下用户自己放的网页一律不动；目录只在变空时 rmdir。
case "$1" in
    purge|0)
        rm -rf /etc/fast-https/cert /etc/fast-https/conf.d
        rm -f /etc/fast-https/fast-https.json /etc/fast-https/mime.json
        rm -rf /var/log/fast-https /run/fast-https
        rmdir /etc/fast-https 2>/dev/null || true

        rm -f /usr/share/fast-https/httpdoc/root/root.crt
        rmdir /usr/share/fast-https/httpdoc/root 2>/dev/null || true
        rmdir /usr/share/fast-https/httpdoc 2>/dev/null || true
        rmdir /usr/share/fast-https 2>/dev/null || true

        echo "fast-https: purged config, certs and logs under /etc/fast-https and /var/log/fast-https"
        ;;
esac

exit 0
