#!/bin/sh
# Alpine 实机验证：安装 → 启动 → 日志轮转 → 卸载（apk 无参数，按设计保留数据）。
# 由 run.sh 在 alpine 容器里执行。
set -u
. /scripts/common.sh

ARCH=${ARCH:-amd64}
PKG=$(ls /dist/fast-https_*_linux_${ARCH}.apk 2>/dev/null | head -1)
if [ -z "$PKG" ]; then bad "dist 下没有 linux/${ARCH} 的 apk"; report; fi

echo "=== 系统 ==="
head -2 /etc/os-release

echo "=== 安装 ==="
apk update >/dev/null 2>&1 || true
if apk add --allow-untrusted "$PKG" >/tmp/install.log 2>&1; then
    pass "apk add"
else
    bad "apk add"
    tail -20 /tmp/install.log
fi
command -v curl >/dev/null 2>&1 || apk add curl >/dev/null 2>&1

echo "=== 包元数据 ==="
apk info -R fast-https || true
echo "=== 文件树 ==="
apk info -L fast-https | sort

expect_present /usr/bin/fast-https "二进制在 /usr/bin"
expect_present /etc/fast-https/fast-https.json "主配置在 /etc/fast-https"
expect_present /etc/fast-https/conf.d/default.json "站点片段在 /etc/fast-https/conf.d"
expect_present /etc/fast-https/cert "证书目录已创建"
expect_present /var/log/fast-https "日志目录在 /var/log/fast-https"
expect_present /etc/logrotate.d/fast-https "logrotate 配置已安装"

# apk 的文件清单不带前导斜杠。
if apk info -L fast-https | grep -q 'usr/share/man/man1/fast-https.1'; then
    pass "man 页已在包内登记"
else
    bad "man 页不在包内"
fi
if [ -e /usr/share/man/man1/fast-https.1 ] || [ -e /usr/share/man/man1/fast-https.1.gz ]; then
    pass "man 页文件已落盘"
else
    echo "INFO  镜像未落盘 man 页（精简镜像常见），不视为打包缺陷"
fi

if command -v logrotate >/dev/null 2>&1; then
    pass "logrotate 已随依赖安装"
    if logrotate -d /etc/logrotate.d/fast-https 2>&1 | grep -q "considering log"; then
        pass "logrotate 配置可解析"
    else
        bad "logrotate 配置解析"
        logrotate -d /etc/logrotate.d/fast-https 2>&1 | tail -5
    fi
else
    bad "logrotate 未安装（轮转配置不会生效）"
fi

echo "=== 运行期路径 ==="
fast-https config path --json || bad "config path"

echo "=== 启动（无 systemd，靠 postinstall 建目录） ==="
if start_server; then pass "start 且 443 已监听"; else bad "start"; fi
if listen_http;  then pass "80 在听";  else bad "80 未监听"; fi
if listen_https; then pass "443 在听"; else bad "443 未监听"; fi
expect_present /run/fast-https/fast-https.pid "pid 写在 /run/fast-https"
expect_present /var/log/fast-https/access.log "访问日志已创建"
expect_present /etc/fast-https/cert/localhost.pem "自签证书已生成"
if curl -sk --max-time 5 https://127.0.0.1/ | grep -qi 'html'; then pass "HTTPS 返回页面"; else bad "HTTPS 无响应"; fi

echo 'USER-PAGE' > /usr/share/fast-https/httpdoc/root/user.html

echo "=== 轮转（copytruncate） ==="
hit
sleep 0.5
before=$(wc -c < /var/log/fast-https/access.log)
if [ "$before" -gt 0 ]; then pass "访问日志已产生内容（${before} 字节）"; else bad "访问日志为空，无法验证轮转"; fi
logrotate -f /etc/logrotate.d/fast-https
expect_present /var/log/fast-https/access.log.1 "轮转出 access.log.1"
if [ "$(wc -c < /var/log/fast-https/access.log)" -lt "$before" ]; then pass "当前文件已截断"; else bad "当前文件未截断"; fi
hit
sleep 0.5
if [ -s /var/log/fast-https/access.log ]; then pass "截断后进程仍在写当前文件"; else bad "截断后无新日志（fd 仍指向旧 inode）"; fi

echo "=== 卸载：apk 拿不到参数，保留数据 ==="
stop_service
if apk del fast-https >/tmp/del.log 2>&1; then pass "apk del"; else bad "apk del"; tail -5 /tmp/del.log; fi
expect_absent /usr/bin/fast-https "卸载后二进制已删"
expect_absent /etc/logrotate.d/fast-https "卸载后 logrotate 配置已删"

# apk 没有 purge 语义：包内的默认配置与空目录随包删除，但程序运行期生成的
# 文件（证书、ACME 账户、日志）不是包内文件，post-deinstall 也不删，所以保留。
expect_absent /etc/fast-https/fast-https.json "卸载后包内默认配置随包删除（apk 无 purge）"
expect_present /etc/fast-https/cert/localhost.pem "卸载后运行期生成的证书保留"
expect_present /var/log/fast-https "卸载后日志目录保留"

if apk info -e fast-https >/dev/null 2>&1; then bad "apk 数据库仍记录 fast-https"; else pass "apk 数据库已清除"; fi

report
