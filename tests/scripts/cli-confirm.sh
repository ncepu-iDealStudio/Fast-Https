#!/usr/bin/env bash
#
# CLI 写命令确认回归：非终端必须显式 --yes；终端里提示 y/N；--dry-run 不提示。
#
# 用法（Linux / WSL / Docker 内均可）：
#   bash tests/scripts/cli-confirm.sh
#   BIN=/usr/local/bin/fast-https bash tests/scripts/cli-confirm.sh
#   KEEP_TMP=1 bash tests/scripts/cli-confirm.sh        # 保留临时目录排查
#
# 依赖：bash、script（util-linux，用来分配伪终端）、grep。
# 未设置 BIN 时用 `go build` 构建当前仓库的 Linux 二进制。
#
# 覆盖：
#   - site update/delete/add、site location add/delete、
#     cache add/update/delete、config proxy、config tls
#   - 非终端（stdin 是管道或 /dev/null）缺 --yes → 退出码 2
#   - 终端回答 n → 取消，配置不变；回答 y → 执行成功
#   - 带 --yes 或 --dry-run → 不出现提示
#
set -uo pipefail

BIN=${BIN:-}
KEEP_TMP=${KEEP_TMP:-0}

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

export FAST_HTTPS_LANG=zh
# 避免 lipgloss/termenv 在伪终端里查询背景色与光标位置（会吞掉预置的 y/N 输入）。
export TERM=dumb
export NO_COLOR=1

WORK=$(mktemp -d 2>/dev/null || mktemp -d -t cli-confirm)
BASE="$WORK/base"
CONFIG="$BASE/config/fast-https.json"
SITE="$BASE/config/conf.d/web.json"
WWW="$BASE/www"

PASSED=0
FAILED=0
CODE=0
OUT=""

cleanup() {
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
	if [ -n "$OUT" ]; then
		printf '%s\n' "$OUT" | sed 's/^/       | /'
	fi
}

die() {
	printf 'fatal: %s\n' "$1" >&2
	exit 2
}

command -v script >/dev/null 2>&1 || die "需要 util-linux 的 script 命令来分配伪终端"

if [ -z "$BIN" ]; then
	command -v go >/dev/null 2>&1 || die "未设置 BIN，且当前环境没有 go"
	BIN="$WORK/fast-https"
	printf 'building %s\n' "$BIN"
	(cd "$REPO_ROOT" && go build -o "$BIN" .) || die "go build 失败"
fi
[ -x "$BIN" ] || die "BIN 不可执行：$BIN"
case $BIN in
/*) ;;
*) BIN=$(cd "$(dirname "$BIN")" && pwd)/$(basename "$BIN") ;;
esac

# ---------- fixture ----------
mkdir -p "$BASE/config/conf.d" "$WWW" "$BASE/state"
[ -f "$REPO_ROOT/config/mime.json" ] && cp "$REPO_ROOT/config/mime.json" "$BASE/config/mime.json"

cat >"$CONFIG" <<EOF
{
  "http": {
    "include": ["$BASE/config/conf.d"]
  }
}
EOF

cat >"$SITE" <<EOF
{
  "listen": 18081,
  "server_name": "web.local",
  "location": [{"url": "/", "type": "local", "root": "$WWW"}]
}
EOF

export FASTHTTPS_STATE_DIR="$BASE/state"

# run_plain：stdin 接 /dev/null（非终端）。
run_plain() {
	OUT=$(cd "$BASE" && "$@" </dev/null 2>&1)
	CODE=$?
	OUT=$(printf '%s' "$OUT" | tr -d '\r')
}

# run_piped：stdin 是管道里的一行 y（仍然不是终端）。
run_piped() {
	local answer=$1
	shift
	OUT=$(cd "$BASE" && printf '%s\n' "$answer" | "$@" 2>&1)
	CODE=$?
	OUT=$(printf '%s' "$OUT" | tr -d '\r')
}

# run_tty：用 script 分配伪终端，再把 answer 喂给子进程。
run_tty() {
	local answer=$1
	shift
	local cmd
	cmd=$(printf '%q ' "$@")
	printf '%s\n' "$answer" >"$WORK/answer"
	(cd "$BASE" && script -qec "$cmd" /dev/null <"$WORK/answer" >"$WORK/tty.out" 2>&1)
	CODE=$?
	OUT=$(tr -d '\r' <"$WORK/tty.out")
}

expect_code() {
	if [ "$CODE" = "$2" ]; then
		ok "$1 (exit $CODE)"
	else
		bad "$1：exit $CODE，期望 $2"
	fi
}

expect_nonzero() {
	if [ "$CODE" -ne 0 ]; then
		ok "$1 (exit $CODE)"
	else
		bad "$1：命令意外成功"
	fi
}

expect_contains() {
	case $OUT in
	*"$2"*) ok "$1 含「$2」" ;;
	*) bad "$1 缺少「$2」" ;;
	esac
}

expect_absent() {
	case $OUT in
	*"$2"*) bad "$1 不应出现「$2」" ;;
	*) ok "$1 未出现「$2」" ;;
	esac
}

expect_file_has() {
	if grep -qF -- "$2" "$1" 2>/dev/null; then
		ok "$3"
	else
		bad "$3（$1 缺少 $2）"
	fi
}

expect_file_lacks() {
	if grep -qF -- "$2" "$1" 2>/dev/null; then
		bad "$3（$1 仍有 $2）"
	else
		ok "$3"
	fi
}

# ---------- 非终端：缺 --yes 必须拒绝 ----------
printf '\n[1] 非终端（/dev/null）：缺 --yes 应退出码 2\n'
run_plain "$BIN" site update web --listen 18082 --config "$CONFIG"
expect_code "site update" 2
expect_contains "site update" "需要显式 --yes"

run_plain "$BIN" site delete web --config "$CONFIG"
expect_code "site delete" 2
expect_contains "site delete" "需要显式 --yes"

run_plain "$BIN" site add --domain blog.local --root "$WWW" --listen 18083 --config "$CONFIG"
expect_code "site add" 2
expect_contains "site add" "需要显式 --yes"

run_plain "$BIN" site location add web --url /api --type proxy --proxy-pass http://127.0.0.1:9000 --config "$CONFIG"
expect_code "site location add" 2
expect_contains "site location add" "需要显式 --yes"

run_plain "$BIN" cache add --name edge --path ./cache/edge --valid any:10s --config "$CONFIG"
expect_code "cache add" 2
expect_contains "cache add" "需要显式 --yes"

run_plain "$BIN" config proxy --max-hops 5 --config "$CONFIG"
expect_code "config proxy" 2
expect_contains "config proxy" "需要显式 --yes"

expect_file_has "$SITE" '"listen": 18081' "拒绝后站点配置未改动"
expect_file_lacks "$CONFIG" '"cache"' "拒绝后主配置未出现 cache 段"

printf '\n[2] 管道输入 y 仍不是终端：必须 --yes\n'
run_piped y "$BIN" site delete web --config "$CONFIG"
expect_code "echo y | site delete" 2
expect_contains "echo y | site delete" "需要显式 --yes"
[ -f "$SITE" ] && ok "站点文件仍在" || bad "站点文件被误删"

# ---------- 终端：n 取消、y 执行 ----------
printf '\n[3] 终端回答 n：取消且不落盘\n'
run_tty n "$BIN" site update web --listen 18082 --config "$CONFIG"
expect_nonzero "site update 取消"
expect_contains "site update 取消" "继续？"
expect_contains "site update 取消" "已取消"
expect_file_has "$SITE" '"listen": 18081' "取消后 listen 仍为 18081"

run_tty n "$BIN" site update web --listen 18082 --json --config "$CONFIG"
expect_contains "取消 JSON" '"code":"CANCELLED"'

run_tty n "$BIN" cache add --name edge --path ./cache/edge --valid any:10s --config "$CONFIG"
expect_nonzero "cache add 取消"
expect_file_lacks "$CONFIG" '"cache"' "取消后没有 cache 段"

run_tty n "$BIN" site location add web --url /api --type proxy --proxy-pass http://127.0.0.1:9000 --config "$CONFIG"
expect_nonzero "site location add 取消"
expect_file_lacks "$SITE" '"/api"' "取消后没有 /api"

run_tty n "$BIN" config proxy --max-hops 5 --config "$CONFIG"
expect_nonzero "config proxy 取消"
expect_file_lacks "$CONFIG" '"proxy"' "取消后没有 proxy 段"

printf '\n[4] 终端回答 y：执行成功\n'
run_tty y "$BIN" site update web --listen 18082 --config "$CONFIG"
expect_code "site update 确认" 0
expect_file_has "$SITE" '18082' "listen 已改为 18082"

run_tty y "$BIN" cache add --name edge --path ./cache/edge --valid any:10s --config "$CONFIG"
expect_code "cache add 确认" 0
expect_file_has "$CONFIG" '"edge"' "cache 区已写入"

run_tty y "$BIN" cache update edge --max-size 2048 --config "$CONFIG"
expect_code "cache update 确认" 0
expect_file_has "$CONFIG" '2048' "max_size 已更新"

run_tty n "$BIN" cache delete edge --config "$CONFIG"
expect_nonzero "cache delete 取消"
expect_file_has "$CONFIG" '"edge"' "取消后缓存区仍在"
run_tty y "$BIN" cache delete edge --config "$CONFIG"
expect_code "cache delete 确认" 0
expect_file_lacks "$CONFIG" '"edge"' "确认后缓存区已删除"

run_tty y "$BIN" site location add web --url /api --type proxy --proxy-pass http://127.0.0.1:9000 --config "$CONFIG"
expect_code "site location add 确认" 0
expect_file_has "$SITE" '"/api"' "额外 location 已写入"
run_tty n "$BIN" site location delete web --url /api --config "$CONFIG"
expect_nonzero "site location delete 取消"
expect_file_has "$SITE" '"/api"' "取消后 /api 仍在"
run_tty y "$BIN" site location delete web --url /api --config "$CONFIG"
expect_code "site location delete 确认" 0
expect_file_lacks "$SITE" '"/api"' "确认后 /api 已删除"

run_tty y "$BIN" config proxy --max-hops 5 --config "$CONFIG"
expect_code "config proxy 确认" 0
expect_file_has "$CONFIG" '"proxy"' "proxy 段已写入"

# ---------- 终端 + --yes / --dry-run：不提示 ----------
printf '\n[5] 终端 + --yes / --dry-run：不再询问\n'
run_tty n "$BIN" site update web --listen 18084 --yes --config "$CONFIG"
expect_code "site update --yes" 0
expect_absent "site update --yes" "继续？"
expect_file_has "$SITE" '18084' "--yes 直接生效"

run_tty n "$BIN" site update web --listen 18085 --dry-run --config "$CONFIG"
expect_code "site update --dry-run" 0
expect_absent "site update --dry-run" "继续？"
expect_file_has "$SITE" '18084' "dry-run 未写盘"

# ---------- 站点新增 / 删除 ----------
printf '\n[6] site add / site delete 的终端确认\n'
run_tty n "$BIN" site add --domain blog.local --root "$WWW" --listen 18083 --config "$CONFIG"
expect_nonzero "site add 取消"
[ -f "$BASE/config/conf.d/blog.local.json" ] && bad "取消后不应创建 blog.local.json" || ok "取消后未创建站点文件"

run_tty y "$BIN" site add --domain blog.local --root "$WWW" --listen 18083 --config "$CONFIG"
expect_code "site add 确认" 0
[ -f "$BASE/config/conf.d/blog.local.json" ] && ok "站点文件已创建" || bad "站点文件未创建"

run_tty n "$BIN" site delete web --config "$CONFIG"
expect_nonzero "site delete 取消"
[ -f "$SITE" ] && ok "取消后站点文件仍在" || bad "取消后站点文件被删除"

run_tty y "$BIN" site delete web --config "$CONFIG"
expect_code "site delete 确认" 0
[ -f "$SITE" ] && bad "确认后站点文件应删除" || ok "确认后站点文件已删除"

# ---------- config tls（需要 openssl 生成真实证书） ----------
printf '\n[7] config tls 的终端确认\n'
if command -v openssl >/dev/null 2>&1; then
	openssl req -x509 -nodes -newkey rsa:2048 -days 1 \
		-keyout "$BASE/def.key" -out "$BASE/def.pem" -subj "/CN=default.local" >/dev/null 2>&1 \
		|| die "openssl 生成测试证书失败"

	run_tty n "$BIN" config tls --cert "$BASE/def.pem" --key "$BASE/def.key" --config "$CONFIG"
	expect_nonzero "config tls 取消"
	expect_file_lacks "$CONFIG" 'default_certificate' "取消后没有 default_certificate"

	run_tty y "$BIN" config tls --cert "$BASE/def.pem" --key "$BASE/def.key" --config "$CONFIG"
	expect_code "config tls 确认" 0
	expect_file_has "$CONFIG" 'default_certificate' "default_certificate 已写入"
else
	printf '  skip 未安装 openssl，跳过 config tls\n'
fi

# ---------- config versions delete ----------
printf '\n[8] config versions delete 的确认与 dry-run\n'
run_tty y "$BIN" config proxy --max-hops 6 --config "$CONFIG"
expect_code "准备版本快照 1" 0
run_tty y "$BIN" config proxy --max-hops 7 --config "$CONFIG"
expect_code "准备版本快照 2" 0

run_plain "$BIN" config versions --json --config "$CONFIG"
VERSION=$(printf '%s' "$OUT" | grep -o '"version":"[^"]*"' | tail -n 1 | cut -d'"' -f4)
[ -n "$VERSION" ] || die "无法从 config versions 输出解析版本号：$OUT"
SNAPDIR="$BASE/state/snapshots/$VERSION"
[ -d "$SNAPDIR" ] || die "快照目录不存在：$SNAPDIR"

run_plain "$BIN" config versions delete "$VERSION" --config "$CONFIG"
expect_code "versions delete" 2
expect_contains "versions delete" "需要显式 --yes"

run_plain "$BIN" config versions delete "$VERSION" --dry-run --config "$CONFIG"
expect_code "versions delete --dry-run" 0
expect_contains "versions delete --dry-run" "dry-run"
[ -d "$SNAPDIR" ] && ok "dry-run 未删除快照" || bad "dry-run 删除了快照"

run_tty n "$BIN" config versions delete "$VERSION" --config "$CONFIG"
expect_nonzero "versions delete 取消"
expect_contains "versions delete 取消" "已取消"
[ -d "$SNAPDIR" ] && ok "取消后快照仍在" || bad "取消后快照被删除"

run_tty y "$BIN" config versions delete "$VERSION" --config "$CONFIG"
expect_code "versions delete 确认" 0
[ -d "$SNAPDIR" ] && bad "确认后快照应删除" || ok "确认后快照已删除"

# ---------- acme setup 非交互 ----------
printf '\n[9] acme setup 非交互的确认\n'
run_plain "$BIN" acme setup --email ops@example.com --config "$CONFIG"
expect_code "acme setup" 2
expect_contains "acme setup" "需要显式 --yes"
expect_file_lacks "$CONFIG" '"acme"' "拒绝后没有 acme 段"

run_plain "$BIN" acme setup --email ops@example.com --dry-run --config "$CONFIG"
expect_code "acme setup --dry-run" 0
expect_file_lacks "$CONFIG" '"acme"' "dry-run 未写入 acme 段"

run_tty n "$BIN" acme setup --email ops@example.com --non-interactive --config "$CONFIG"
expect_nonzero "acme setup 取消"
expect_file_lacks "$CONFIG" '"acme"' "取消后没有 acme 段"

run_tty y "$BIN" acme setup --email ops@example.com --non-interactive --config "$CONFIG"
expect_code "acme setup 确认" 0
expect_file_has "$CONFIG" '"acme"' "确认后 acme 段已写入"

printf '\n结果：%d 通过，%d 失败\n' "$PASSED" "$FAILED"
[ "$FAILED" -eq 0 ] || exit 1
exit 0
