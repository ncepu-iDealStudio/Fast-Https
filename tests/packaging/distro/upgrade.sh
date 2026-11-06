#!/bin/sh
# 升级迁移实机验证：旧版单前缀布局（/usr/share/fast-https/config）
# 安装新版 deb 后，配置与证书应被复制到 /etc/fast-https，旧文件保留不删。
# 由 run.sh 在 ubuntu 容器里执行。
set -u
. /scripts/common.sh
export DEBIAN_FRONTEND=noninteractive

ARCH=${ARCH:-amd64}
PKG=$(ls /dist/fast-https_*_linux_${ARCH}.deb 2>/dev/null | head -1)
if [ -z "$PKG" ]; then bad "dist 下没有 linux/${ARCH} 的 deb"; report; fi

LEGACY=/usr/share/fast-https/config

echo "=== 构造旧版布局 ==="
mkdir -p "$LEGACY/conf.d" "$LEGACY/cert/acme" /usr/share/fast-https/httpdoc/root
cat > "$LEGACY/fast-https.json" <<'EOF'
{"http":{"servers_limit":{"limit":42}}}
EOF
cat > "$LEGACY/conf.d/legacy-site.json" <<'EOF'
{"listen":[8080],"server_name":"legacy","location":[{"url":"/","type":"local","root":"./httpdoc/root","index":["index.html"]}]}
EOF
echo 'LEGACY-SITE-CERT' > "$LEGACY/cert/mysite.pem"
echo 'ACME-ACCOUNT-KEY' > "$LEGACY/cert/acme/account.key"
echo 'SITE-INDEX' > /usr/share/fast-https/httpdoc/root/index.html
pass "旧布局已就绪"

echo "=== 安装新版 ==="
apt-get update -qq
if apt-get install -y -qq "$PKG" >/tmp/install.log 2>&1; then pass "apt install"; else bad "apt install"; tail -20 /tmp/install.log; fi
command -v curl >/dev/null 2>&1 || apt-get install -y -qq curl

echo "=== 迁移结果 ==="
expect_present /etc/fast-https/cert/mysite.pem "包内没有的证书文件被迁移"
expect_present /etc/fast-https/conf.d/legacy-site.json "旧站点片段被迁移"
expect_present /etc/fast-https/cert/acme/account.key "ACME 账户私钥被迁移（不重新签发）"
expect_present "$LEGACY/cert/mysite.pem" "旧文件保留（只复制不删除）"
expect_present "$LEGACY/fast-https.json" "旧主配置保留"
expect_present /usr/share/fast-https/httpdoc/root/index.html "站点内容原地保留"

if grep -q '"limit":42' /etc/fast-https/fast-https.json 2>/dev/null; then
    bad "主配置被旧文件覆盖（应保留包内新默认值，由管理员自行合并）"
else
    pass "主配置使用包内默认值，旧配置未被盲目覆盖"
fi

echo "=== 迁移幂等：再次安装不覆盖已有文件 ==="
echo 'ADMIN-EDITED' > /etc/fast-https/cert/mysite.pem
if dpkg -i "$PKG" >/tmp/reinstall.log 2>&1; then pass "重装"; else bad "重装"; tail -10 /tmp/reinstall.log; fi
if [ "$(cat /etc/fast-https/cert/mysite.pem)" = "ADMIN-EDITED" ]; then
    pass "目标已存在时迁移跳过，不覆盖现场文件"
else
    bad "重装时迁移覆盖了 /etc 下的现有文件"
fi
if [ "$(cat "$LEGACY/cert/mysite.pem")" = "LEGACY-SITE-CERT" ]; then
    pass "旧文件始终只读不写"
else
    bad "旧文件被修改"
fi

echo "=== 迁移后配置可用 ==="
if fast-https config path --json >/tmp/paths.log 2>&1; then pass "config path"; else bad "config path"; cat /tmp/paths.log; fi
# 不再单独校验：start 的预检就是同一项检查，能起来说明迁移后的配置可加载。
if start_server; then pass "迁移后服务可启动并监听 443（启动预检已校验配置）"; else bad "迁移后服务启动失败"; fi
hit
if [ -s /var/log/fast-https/access.log ]; then pass "迁移后服务可正常写日志"; else bad "迁移后无访问日志"; fi

echo "=== 清理 ==="
stop_service
apt-get purge -y -qq fast-https >/dev/null 2>&1 || true
expect_absent /etc/fast-https/cert "purge 后 /etc 下证书已删"
expect_present "$LEGACY/cert/mysite.pem" "purge 不动旧布局目录（需管理员手工清理）"

report
