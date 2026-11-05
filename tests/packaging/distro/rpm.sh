#!/bin/sh
# RHEL 系实机验证（AlmaLinux）：安装 → 启动 → 日志轮转 → 重装保留本地配置 → 卸载清理。
# 由 run.sh 在 almalinux 容器里执行。
set -u
. /scripts/common.sh

ARCH=${ARCH:-amd64}
PKG=$(ls /dist/fast-https_*_linux_${ARCH}.rpm 2>/dev/null | head -1)
if [ -z "$PKG" ]; then bad "dist 下没有 linux/${ARCH} 的 rpm"; report; fi

echo "=== 系统 ==="
head -2 /etc/os-release

echo "=== 安装 ==="
if dnf install -y "$PKG" >/tmp/install.log 2>&1; then
    pass "dnf install"
else
    bad "dnf install"
    tail -20 /tmp/install.log
fi
# curl-minimal 与 curl 冲突，已有 curl 就别装。
command -v curl >/dev/null 2>&1 || dnf install -y curl >/dev/null 2>&1

echo "=== 包元数据 ==="
rpm -q --requires fast-https | grep -v rpmlib || true
echo "--- 配置文件 ---"
rpm -qc fast-https || true
echo "=== 文件树 ==="
rpm -ql fast-https | sort

expect_present /usr/bin/fast-https "二进制在 /usr/bin"
expect_present /etc/fast-https/fast-https.json "主配置在 /etc/fast-https"
expect_present /etc/fast-https/conf.d/default.json "站点片段在 /etc/fast-https/conf.d"
expect_present /etc/fast-https/cert "证书目录已创建"
expect_present /var/log/fast-https "日志目录在 /var/log/fast-https"
expect_present /etc/logrotate.d/fast-https "logrotate 配置已安装"
expect_absent /run/fast-https/fast-https.pid "rpm 包内不含 /run 下的 pid"

if rpm -ql fast-https | grep -q '/usr/share/man/man1/fast-https.1'; then
    pass "man 页已在包内登记"
else
    bad "man 页不在包内"
fi
if [ -e /usr/share/man/man1/fast-https.1 ] || [ -e /usr/share/man/man1/fast-https.1.gz ]; then
    pass "man 页文件已落盘"
else
    echo "INFO  镜像未落盘 man 页（精简镜像常见），不视为打包缺陷"
fi

if rpm -qc fast-https | grep -qx '/etc/logrotate.d/fast-https'; then
    pass "logrotate 配置登记为配置文件"
else
    bad "logrotate 配置未登记为配置文件（升级会被覆盖）"
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

echo "=== 重装：%config(noreplace) 保留本地修改 ==="
stop_service
echo '{"http":{"servers_limit":{"limit":7}}}' > /etc/fast-https/fast-https.json
if rpm -U --replacepkgs "$PKG" >/tmp/reinstall.log 2>&1; then
    pass "同版本重装"
else
    bad "同版本重装"
    tail -5 /tmp/reinstall.log
fi
if grep -q '"limit":7' /etc/fast-https/fast-https.json; then
    pass "重装后本地配置保留（noreplace 生效）"
else
    bad "重装后本地配置被覆盖"
fi
if [ -e /etc/fast-https/fast-https.json.rpmnew ]; then
    pass "新默认值落为 .rpmnew（管理员可对比）"
else
    echo "INFO  未生成 .rpmnew（包内新配置与本机相同或 rpm 未发现差异）"
fi

echo "=== 卸载：rpm -e 清理 ==="
if rpm -e fast-https >/tmp/erase.log 2>&1; then pass "rpm -e"; else bad "rpm -e"; tail -5 /tmp/erase.log; fi
expect_absent /etc/fast-https/cert "卸载后证书目录已删"
expect_absent /etc/fast-https/conf.d "卸载后站点片段目录已删"
expect_absent /var/log/fast-https "卸载后日志目录已删"
expect_absent /run/fast-https "卸载后 run 目录已删"
expect_absent /etc/logrotate.d/fast-https "卸载后 logrotate 配置已删"
expect_absent /usr/share/fast-https/httpdoc/root/root.crt "卸载后孤儿 CA 证书已删"
expect_absent /usr/bin/fast-https "卸载后二进制已删"
expect_present /usr/share/fast-https/httpdoc/root/user.html "卸载保留用户自建网页"
if rpm -q fast-https >/dev/null 2>&1; then bad "rpm 数据库仍记录 fast-https"; else pass "rpm 数据库已清除"; fi

# rpm 对修改过的 %config 可能留下 .rpmsave，属正常行为，但目录不该整体残留。
if [ -d /etc/fast-https ]; then
    echo "INFO  /etc/fast-https 残留内容：$(ls -A /etc/fast-https | tr '\n' ' ')"
fi

report
