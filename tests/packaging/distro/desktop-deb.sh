#!/bin/sh
# 桌面端 deb 的离线单包实机验证：只装 fast-https-desktop_*.deb，不装主程序包。
# 重点是验证「装完能不能真的把服务跑起来」——包里自带的 fast-https 必须和
# 独立主程序包是同一个二进制、同一个 /usr/bin 路径，且运行期资源齐备。
# 由 run.sh 在 ubuntu:24.04 容器里执行，/dist 是宿主机 dist/，/scripts 是本目录。
set -u
. /scripts/common.sh
export DEBIAN_FRONTEND=noninteractive

ARCH=${ARCH:-amd64}
PKG=$(ls /dist/fast-https-desktop_*_linux_${ARCH}.deb 2>/dev/null | head -1)
MAIN=$(ls /dist/fast-https_*_linux_${ARCH}.deb 2>/dev/null | head -1)
if [ -z "$PKG" ]; then bad "dist 下没有 desktop 的 linux/${ARCH} deb"; report; fi

echo "=== 包 ==="
echo "INFO  desktop=$PKG"
echo "INFO  main=$MAIN"
echo "=== control ==="
dpkg-deb -f "$PKG" Package Version Depends Conflicts Replaces

echo "=== 安装（离线单包：只装 desktop deb）==="
apt-get update -qq
apt-get install -y -qq --no-install-recommends "$PKG" || bad "apt install desktop deb"
command -v curl >/dev/null 2>&1 || apt-get install -y -qq curl

echo "=== dpkg 状态 ==="
dpkg-query -W -f='${Package} ${Status} ${Version}\n' fast-https-desktop 2>&1
dpkg-query -W -f='${Package} ${Status}\n' fast-https 2>&1 || echo "INFO  独立主程序包未安装（离线单包场景）"

echo "=== 文件 ==="
expect_present /usr/bin/fast-https "主程序在 /usr/bin"
expect_present /usr/bin/fast-https-desktop "桌面端在 /usr/bin"
if [ -x /usr/bin/fast-https ]; then pass "主程序可执行"; else bad "主程序不可执行"; fi
if [ -x /usr/bin/fast-https-desktop ]; then pass "桌面端可执行"; else bad "桌面端不可执行"; fi
expect_present /usr/share/applications/fast-https-desktop.desktop ".desktop 已安装"
expect_present /usr/share/icons/hicolor/256x256/apps/fast-https-desktop.png "图标已安装"
expect_absent /usr/lib/fast-https/fast-https "没有私有路径下的主程序副本"

echo "=== 与独立主程序包二进制一致性 ==="
if [ -n "$MAIN" ]; then
    mkdir -p /tmp/mainpkg
    dpkg-deb -x "$MAIN" /tmp/mainpkg
    if cmp -s /tmp/mainpkg/usr/bin/fast-https /usr/bin/fast-https; then
        pass "两份 /usr/bin/fast-https 字节一致"
    else
        bad "自带主程序与独立主程序包不一致"
    fi
else
    bad "dist 下没有独立主程序 deb，无法对比"
fi

echo "=== 运行期资源（与独立主程序包一致）==="
expect_present /etc/fast-https/fast-https.json "主配置"
expect_present /etc/fast-https/conf.d/default.json "站点片段"
expect_present /etc/fast-https/cert/.keep "cert 占位"
expect_present /usr/share/fast-https/httpdoc/root/index.html "默认站点"
expect_present /usr/lib/systemd/system/fast-https.service "systemd unit"
expect_present /etc/logrotate.d/fast-https "logrotate 配置"
expect_present /var/log/fast-https "日志目录"

echo "=== 主程序命令 ==="
fast-https version || bad "fast-https version"
fast-https config path --json || bad "config path --json"

echo "=== 启动服务 ==="
if start_server; then pass "start 且 443 已监听"; else bad "start 失败"; fi
if listen_http; then pass "80 在听"; else bad "80 未监听"; fi
if listen_https; then pass "443 在听"; else bad "443 未监听"; fi
expect_present /run/fast-https/fast-https.pid "pid 写在 /run/fast-https"
expect_present /var/log/fast-https/access.log "访问日志已创建"

echo "=== 卸载清理 ==="
stop_service
apt-get remove -y -qq fast-https-desktop
expect_absent /usr/bin/fast-https "卸载后主程序已删"
expect_absent /usr/bin/fast-https-desktop "卸载后桌面端已删"
if dpkg --audit | grep -q .; then bad "dpkg --audit 有输出"; dpkg --audit; else pass "dpkg 数据库干净"; fi

echo "=== 替换场景：已装独立主程序包时再装 desktop 包 ==="
if [ -n "$MAIN" ]; then
    apt-get install -y -qq --no-install-recommends "$MAIN" || bad "安装独立主程序包"
    echo 'KEEP-THIS' > /etc/fast-https/conf.d/local.json
    apt-get install -y -qq --no-install-recommends "$PKG" || bad "desktop 包替换主程序包"
    expect_present /usr/bin/fast-https "替换后主程序仍在"
    expect_present /usr/bin/fast-https-desktop "替换后桌面端已装"
    expect_present /etc/fast-https/conf.d/local.json "替换后现场配置保留"
    if dpkg-query -W -f='${Status}' fast-https 2>/dev/null | grep -q installed; then
        bad "独立主程序包仍装着（两个包会抢同一文件）"
    else
        pass "独立主程序包已被替换，/usr/bin/fast-https 最终只有一份"
    fi
    if dpkg --audit | grep -q .; then bad "替换后 dpkg --audit 有输出"; dpkg --audit; else pass "替换后 dpkg 数据库干净"; fi
    stop_service
    apt-get purge -y -qq fast-https-desktop >/dev/null 2>&1 || true
fi

report
