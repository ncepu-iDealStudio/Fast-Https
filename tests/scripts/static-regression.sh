#!/usr/bin/env bash
#
# 静态资源 curl 回归：文件访问、Content-Type、带哈希文件、敏感路径、
# 目录请求（301 / 不列目录）与 404。任何一条不符合预期即以非 0 退出。
#
# 用法：
#   bash tests/scripts/static-regression.sh                      # 自建临时实例
#   BASE_URL=http://127.0.0.1:8080 bash tests/scripts/static-regression.sh
#   BIN=/usr/local/bin/fast-https bash tests/scripts/static-regression.sh
#   SKIP_SERVER=1 BASE_URL=http://127.0.0.1:8080 bash tests/scripts/static-regression.sh
#
# 环境变量：
#   BASE_URL     请求地址，默认 http://127.0.0.1:18080（host:port 决定临时实例监听端口）
#   BIN          被测二进制；未设置时用 `go build` 构建到临时目录
#   SKIP_SERVER  1 表示不起实例，只对 BASE_URL 发请求。此时目标必须已经
#                按下面的「fixture 布局」在提供服务（用于验证线上实例）。
#   KEEP_TMP     1 表示保留临时目录（排查启动失败时用）
#
# fixture 布局（自建实例时脚本自动创建）：
#   /index.html                        text/html
#   /plain.txt                         text/plain
#   /assets/app.3f9a2c1b.js            application/javascript（带哈希文件名）
#   /docs/index.html                   目录 + 索引页
#   /noindex/                          目录无索引页，不得列出内容
#   /.git/config                       含哨兵串，必须取不到
#   <站外>/secret/outside.txt          含哨兵串，`..` 与编码穿越都必须拒绝
#
set -uo pipefail

BASE_URL=${BASE_URL:-http://127.0.0.1:18080}
BIN=${BIN:-}
SKIP_SERVER=${SKIP_SERVER:-0}
KEEP_TMP=${KEEP_TMP:-0}

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

GIT_CANARY="CANARY-GIT-CONFIG-MUST-NOT-LEAK"
OUTSIDE_CANARY="CANARY-OUTSIDE-ROOT-MUST-NOT-LEAK"

WORK=$(mktemp -d 2>/dev/null || mktemp -d -t static-regression)
SERVER_PID=""
FAILED=0
PASSED=0

cleanup() {
	if [ -n "$SERVER_PID" ]; then
		kill "$SERVER_PID" 2>/dev/null
		wait "$SERVER_PID" 2>/dev/null
	fi
	if [ "$KEEP_TMP" = "1" ]; then
		echo "临时目录保留在 $WORK"
	else
		rm -rf "$WORK"
	fi
}
trap cleanup EXIT INT TERM

ok() {
	PASSED=$((PASSED + 1))
	printf '  ok   %s\n' "$1"
}

bad() {
	FAILED=$((FAILED + 1))
	printf '  FAIL %s\n' "$1"
}

die() {
	printf 'fatal: %s\n' "$1" >&2
	exit 2
}

command -v curl >/dev/null 2>&1 || die "需要 curl"

# native_path 把 Git Bash 的 /tmp/... 转成 Windows 可识别的路径。
# 配置里的路径是给 Go 进程用的，不能直接塞 MSYS 风格路径（会被当成 C:\tmp）。
native_path() {
	if command -v cygpath >/dev/null 2>&1; then
		cygpath -m "$1"
	else
		printf '%s' "$1"
	fi
}

# ---------- URL 解析 ----------
rest=${BASE_URL#*://}
hostport=${rest%%/*}
case $hostport in
*:*)
	HOST=${hostport%%:*}
	PORT=${hostport##*:}
	;;
*)
	HOST=$hostport
	case $BASE_URL in
	https://*) PORT=443 ;;
	*) PORT=80 ;;
	esac
	;;
esac

# ---------- 临时实例 ----------
start_server() {
	DOCROOT="$WORK/base/httpdoc/root"
	OUTSIDE="$WORK/outside"

	mkdir -p "$WORK/base/config" "$DOCROOT/assets" "$DOCROOT/docs" "$DOCROOT/noindex" \
		"$DOCROOT/.git" "$OUTSIDE/secret"

	printf '<h1>static-regression</h1>\n' >"$DOCROOT/index.html"
	printf 'plain-body\n' >"$DOCROOT/plain.txt"
	printf "console.log('hashed-asset');\n" >"$DOCROOT/assets/app.3f9a2c1b.js"
	printf 'docs-index\n' >"$DOCROOT/docs/index.html"
	printf 'no-listing-please\n' >"$DOCROOT/noindex/readme.txt"
	printf '%s\n' "$GIT_CANARY" >"$DOCROOT/.git/config"
	printf '%s\n' "$OUTSIDE_CANARY" >"$OUTSIDE/secret/outside.txt"

	[ -f "$REPO_ROOT/config/mime.json" ] || die "缺少 $REPO_ROOT/config/mime.json（Content-Type 依赖它）"
	cp "$REPO_ROOT/config/mime.json" "$WORK/base/config/mime.json"

	ROOT_CFG=$(native_path "$DOCROOT")
	cat >"$WORK/base/config/fast-https.json" <<EOF
{
  "http": {
    "server": [
      {
        "listen": "$PORT",
        "server_name": "static.local",
        "location": [
          {
            "url": "/",
            "type": "local",
            "root": "$ROOT_CFG",
            "index": ["index.html"]
          }
        ]
      }
    ]
  }
}
EOF

	if [ -z "$BIN" ]; then
		BIN="$WORK/fast-https"
		printf 'building %s\n' "$BIN"
		(cd "$REPO_ROOT" && go build -o "$BIN" .) || die "go build 失败"
	fi
	[ -x "$BIN" ] || die "BIN 不可执行：$BIN"

	# 前台模式：进程就是脚本持有的那个，最后的 kill 才打得到服务本身。
	(cd "$WORK/base" && FASTHTTPS_FOREGROUND=1 exec "$BIN" start) >"$WORK/server.log" 2>&1 &
	SERVER_PID=$!

	local i code
	for i in $(seq 1 150); do
		code=$(curl -s -o /dev/null -m 2 -w '%{http_code}' "$BASE_URL/" 2>/dev/null)
		if [ "$code" = "200" ]; then
			printf 'server ready at %s (pid %s)\n' "$BASE_URL" "$SERVER_PID"
			return 0
		fi
		if ! kill -0 "$SERVER_PID" 2>/dev/null; then
			tail -n 20 "$WORK/server.log" >&2
			die "服务进程已退出，见上面的日志"
		fi
		sleep 0.2
	done
	tail -n 20 "$WORK/server.log" >&2
	die "30 秒内没有等到 $BASE_URL/ 返回 200"
}

if [ "$SKIP_SERVER" != "1" ]; then
	start_server
else
	printf 'SKIP_SERVER=1：直接测试 %s\n' "$BASE_URL"
fi

# ---------- 断言 ----------
LAST_STATUS="000"
LAST_BODY=""
LAST_HEADERS=""

do_request() {
	LAST_STATUS=$(curl -sS -m 10 --path-as-is -o "$WORK/body" -D "$WORK/headers" -w '%{http_code}' "$1" 2>/dev/null)
	[ -n "$LAST_STATUS" ] || LAST_STATUS="000"
	LAST_BODY=$(cat "$WORK/body" 2>/dev/null)
	LAST_HEADERS=$(cat "$WORK/headers" 2>/dev/null)
}

# status_matches 实际状态 期望状态（可用 | 表示多种，如 "403|404"）
status_matches() {
	local got=$1 want=$2 part
	local IFS='|'
	for part in $want; do
		[ "$got" = "$part" ] && return 0
	done
	return 1
}

# expect_status 名称 期望状态（可用 | 表示多种） URL
expect_status() {
	local name=$1 want=$2 url=$3
	do_request "$url"
	if status_matches "$LAST_STATUS" "$want"; then
		ok "$name -> $LAST_STATUS"
	else
		bad "$name -> $LAST_STATUS，期望 $want（$url）"
	fi
}

expect_body() {
	local name=$1 want=$2
	case $LAST_BODY in
	*"$want"*) ok "$name 含 $want" ;;
	*) bad "$name 缺少 $want，实际正文：$(printf '%s' "$LAST_BODY" | head -c 120)" ;;
	esac
}

expect_body_absent() {
	local name=$1 unwanted=$2
	case $LAST_BODY in
	*"$unwanted"*) bad "$name 泄漏了 $unwanted" ;;
	*) ok "$name 未泄漏 $unwanted" ;;
	esac
}

# expect_content_type 名称 期望前缀（大小写不敏感）
expect_content_type() {
	local name=$1 want=$2
	local got
	got=$(printf '%s' "$LAST_HEADERS" | tr -d '\r' | awk 'tolower($1)=="content-type:"{print $2}' | head -n 1)
	if [ "$got" = "$want" ]; then
		ok "$name Content-Type: $got"
	else
		bad "$name Content-Type: ${got:-<缺失>}，期望 $want"
	fi
}

expect_header_contains() {
	local name=$1 want=$2
	if printf '%s' "$LAST_HEADERS" | tr -d '\r' | grep -qiF -- "$want"; then
		ok "$name 含头部 $want"
	else
		bad "$name 缺少头部 $want"
	fi
}

printf '\n[1] 文件访问与 Content-Type\n'
expect_status "GET /plain.txt" "200" "$BASE_URL/plain.txt"
expect_body "GET /plain.txt" "plain-body"
expect_content_type "GET /plain.txt" "text/plain"

expect_status "GET /" "200" "$BASE_URL/"
expect_body "GET /" "static-regression"
expect_content_type "GET /" "text/html"

printf '\n[2] 带哈希文件名的静态资源\n'
expect_status "GET /assets/app.3f9a2c1b.js" "200" "$BASE_URL/assets/app.3f9a2c1b.js"
expect_body "GET /assets/app.3f9a2c1b.js" "hashed-asset"
expect_content_type "GET /assets/app.3f9a2c1b.js" "application/javascript"

printf '\n[3] 目录请求\n'
expect_status "GET /docs（缺结尾斜杠）" "301" "$BASE_URL/docs"
expect_header_contains "GET /docs" "Location: /docs/"
expect_status "GET /docs/（索引页）" "200" "$BASE_URL/docs/"
expect_body "GET /docs/" "docs-index"
# 目录没有 index 时不得列目录：nginx 回 403，本实现按 404 处理，两者都算通过
expect_status "GET /noindex/（不列目录）" "403|404" "$BASE_URL/noindex/"

printf '\n[4] 404\n'
expect_status "GET /nope.txt" "404" "$BASE_URL/nope.txt"

printf '\n[5] 敏感路径\n'
expect_status "GET /.git/config" "404" "$BASE_URL/.git/config"
expect_body_absent "GET /.git/config" "$GIT_CANARY"

# %2f 编码的穿越：--path-as-is 保证 curl 不替服务端做规范化
expect_status "GET /..%2f..%2fsecret/outside.txt" "400|403|404" "$BASE_URL/..%2f..%2fsecret/outside.txt"
expect_body_absent "GET /..%2f..%2fsecret/outside.txt" "$OUTSIDE_CANARY"

expect_status "GET /../secret/outside.txt" "400|403|404" "$BASE_URL/../secret/outside.txt"
expect_body_absent "GET /../secret/outside.txt" "$OUTSIDE_CANARY"

# 站内的 .. 规范化后仍在根内：允许 200，但不能逃逸到站外
expect_status "GET /docs/../plain.txt" "200" "$BASE_URL/docs/../plain.txt"
expect_body "GET /docs/../plain.txt" "plain-body"

printf '\n结果：%d 通过，%d 失败\n' "$PASSED" "$FAILED"
if [ "$FAILED" -gt 0 ]; then
	exit 1
fi
exit 0
