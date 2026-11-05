#!/bin/sh
# Debian 系实机验证：安装 → 启动 → 日志轮转 → remove 保留 → purge 清理。
# 由 run.sh 在 ubuntu 容器里执行，/dist 是宿主机的 dist/，/scripts 是本目录。
set -u
. /scripts/common.sh
export DEBIAN_FRONTEND=noninteractive

ARCH=${ARCH:-amd64}
PKG=$(ls /dist/fast-https_*_linux_${ARCH}.deb 2>/dev/null | head -1)
if [ -z "$PKG" ]; then bad "dist 下没有 linux/${ARCH} 的 deb"; report; fi

apt-get update -qq
apt-get install -y -qq "$PKG"
# 精简镜像没有 curl，用它探测端口；已有就不动，避免多装依赖。
command -v curl >/dev/null 2>&1 || apt-get install -y -qq curl

echo "=== 包元数据 ==="
dpkg-query -W -f='Depends=${Depends}\n' fast-https
dpkg-query -W -f='${Conffiles}\n' fast-https
echo "=== 文件树 ==="
dpkg -L fast-https | sort

expect_present /usr/bin/fast-https "二进制在 /usr/bin"
expect_present /etc/fast-https/fast-https.json "主配置在 /etc/fast-https"
expect_present /etc/fast-https/conf.d/default.json "站点片段在 /etc/fast-https/conf.d"
expect_present /etc/fast-https/cert/.keep "cert 下的 .keep 占位已安装"
expect_present /var/log/fast-https "日志目录在 /var/log/fast-https"
expect_present /etc/logrotate.d/fast-https "logrotate 配置已安装"
expect_absent /run/fast-https/fast-https.pid "包内不含 /run 下的 pid"

# 官方 ubuntu 镜像带 path-exclude=/usr/share/man/*，man 页不会落盘。
# 所以先确认包内登记，再单独判断是否落盘，避免把镜像策略当成打包缺陷。
if dpkg -L fast-https | grep -q '/usr/share/man/man1/fast-https.1'; then
    pass "man 页已在包内登记"
else
    bad "man 页不在包内"
fi
if [ -e /usr/share/man/man1/fast-https.1 ] || [ -e /usr/share/man/man1/fast-https.1.gz ]; then
    pass "man 页文件已落盘"
else
    echo "INFO  镜像 path-exclude 屏蔽了 man 页，未落盘属镜像策略"
fi

if dpkg-query -W -f='${Conffiles}\n' fast-https | grep -q '/etc/logrotate.d/fast-https'; then
    pass "logrotate 配置登记为 conffile"
else
    bad "logrotate 配置未登记为 conffile（升级会被覆盖）"
fi

echo "=== logrotate 干跑 ==="
logrotate --version | head -1
if logrotate -d /etc/logrotate.d/fast-https 2>&1 | grep -q "considering log"; then
    pass "logrotate 配置可解析"
else
    bad "logrotate 配置解析"
    logrotate -d /etc/logrotate.d/fast-https 2>&1 | tail -5
fi

echo "=== 运行期路径 ==="
fast-https config path --json || bad "config path"

echo "=== 启动（容器里前台运行，脚本自己转后台） ==="
if start_server; then pass "start 且 443 已监听"; else bad "start"; fi
if listen_http;  then pass "80 在听";  else bad "80 未监听"; fi
if listen_https; then pass "443 在听"; else bad "443 未监听"; fi
expect_present /run/fast-https/fast-https.pid "pid 写在 /run/fast-https"
expect_present /var/log/fast-https/access.log "访问日志已创建"
expect_present /etc/fast-https/cert/localhost.pem "自签证书已生成"
if curl -sk --max-time 5 https://127.0.0.1/ | grep -qi 'html'; then pass "HTTPS 返回页面"; else bad "HTTPS 无响应"; fi

# 用户自己放进站点根的文件，任何卸载动作都不该删。
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

echo "=== remove：配置与数据保留 ==="
stop_service
apt-get remove -y -qq fast-https
expect_present /etc/fast-https/fast-https.json "remove 后主配置保留"
expect_present /etc/fast-https/cert/localhost.pem "remove 后证书保留"
expect_present /var/log/fast-https/access.log "remove 后日志保留"
expect_absent /usr/bin/fast-https "remove 后二进制已删"
expect_present /etc/logrotate.d/fast-https "remove 后 logrotate 配置保留（conffile）"

echo "=== purge：清理 ==="
apt-get purge -y -qq fast-https
expect_absent /etc/fast-https/cert "purge 后证书目录已删"
expect_absent /etc/fast-https/conf.d "purge 后站点片段目录已删"
expect_absent /etc/fast-https/fast-https.json "purge 后主配置已删"
expect_absent /var/log/fast-https "purge 后日志目录已删"
expect_absent /run/fast-https "purge 后 run 目录已删"
expect_absent /etc/logrotate.d/fast-https "purge 后 logrotate 配置已删"
expect_absent /usr/share/fast-https/httpdoc/root/root.crt "purge 后孤儿 CA 证书已删"
expect_present /usr/share/fast-https/httpdoc/root/user.html "purge 保留用户自建网页"
if dpkg --audit | grep -q .; then bad "dpkg --audit 有输出"; dpkg --audit; else pass "dpkg 数据库干净"; fi

report
