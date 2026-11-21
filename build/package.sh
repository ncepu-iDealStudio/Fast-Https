#!/usr/bin/env bash
#
# build/package.ps1 的 bash 版本：goreleaser 快照/正式构建、Windows NSIS 安装包，
# 最后把 dist 整理成只剩最终产物。
#
# 适用环境：
#   - Linux / macOS：构建 tar.gz 与 deb/rpm/apk；默认不构建 Windows 桌面端与
#     NSIS 安装包（这两步依赖 Windows 工具链）。需要时用 --installer 尝试，
#     或在 Windows 的 Git Bash 里运行。
#   - Windows Git Bash：行为与 package.ps1 一致，会构建桌面端 exe 与 setup.exe。
#
# 用法：
#   bash build/package.sh                 # 快照构建
#   bash build/package.sh --release       # 正式发版（当前提交必须先打 tag）
#   bash build/package.sh --release --publish
#   bash build/package.sh --skip-build --no-installer
#   bash build/package.sh --installer --makensis /usr/bin/makensis
#
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

GR_CONFIG="build/goreleaser.yaml"

RELEASE=0
PUBLISH=0
SKIP_BUILD=0
NO_INSTALLER=""          # 留空表示按平台取默认值
WITH_DESKTOP=0
MAKENSIS=""

usage() {
	# 只打印文件顶部的注释块（跳过 shebang），遇到第一行非注释就停。
	awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "${BASH_SOURCE[0]}"
}

die() {
	printf 'fatal: %s\n' "$*" >&2
	exit 1
}

warn() {
	printf 'warning: %s\n' "$*" >&2
}

is_windows() {
	case "$(uname -s)" in
	MINGW* | MSYS* | CYGWIN*) return 0 ;;
	*) return 1 ;;
	esac
}

while [ $# -gt 0 ]; do
	case "$1" in
	-r | --release) RELEASE=1 ;;
	-p | --publish) PUBLISH=1 ;;
	-s | --skip-build) SKIP_BUILD=1 ;;
	-n | --no-installer) NO_INSTALLER=1 ;;
	--installer) NO_INSTALLER=0 ;;
	--with-desktop) WITH_DESKTOP=1 ;;
	-m | --makensis)
		shift
		[ $# -gt 0 ] || die "--makensis 需要一个可执行文件路径"
		MAKENSIS=$1
		;;
	-h | --help)
		usage
		exit 0
		;;
	*) die "未知参数：$1（用 --help 查看用法）" ;;
	esac
	shift
done

if [ -z "$NO_INSTALLER" ]; then
	if is_windows; then NO_INSTALLER=0; else NO_INSTALLER=1; fi
fi

# Clear-DistTemp：goreleaser --clean 删 dist 时，残留的 .tmp-* 常被 Docker Desktop
# 或杀软短暂占用。先自己带重试清一遍，清不掉再给出可执行的提示。
clear_dist_temp() {
	local dir=$1 f i
	[ -d "$dir" ] || return 0
	shopt -s nullglob dotglob
	for f in "$dir"/.tmp-*; do
		for i in 1 2 3 4 5 6; do
			rm -rf -- "$f" 2>/dev/null && break
			sleep 0.5
		done
		if [ -e "$f" ]; then
			warn "清不掉 $(basename "$f")：文件仍被占用"
			warn "多半是 Docker Desktop 或杀软正在扫描它。关掉 Docker Desktop 后重跑，或手动删除：$f"
		fi
	done
	shopt -u nullglob dotglob
}

# ConvertTo-Version4：v1.7.1 -> 1.7.1.0，供 NSIS 的 VIProductVersion 使用。
to_version4() {
	awk -v v="$1" 'BEGIN {
		n = split(v, a, /[^0-9]+/)
		out = ""; c = 0
		for (i = 1; i <= n; i++) {
			if (a[i] == "") continue
			if (c < 4) { out = out (c ? "." : "") a[i]; c++ }
		}
		while (c < 4) { out = out ".0"; c++ }
		print out
	}'
}

# NSIS 3 只在脚本带 UTF-8 BOM 时按 UTF-8 解析；否则回落到 ANSI 并在非 ASCII
# 文本上报 "Bad text encoding"。
ensure_bom() {
	local f=$1 first
	[ -f "$f" ] || return 0
	first=$(head -c 3 "$f" | od -An -tx1 | tr -d ' \n')
	if [ "$first" = "efbbbf" ]; then
		return 0
	fi
	printf '\xEF\xBB\xBF' >"$f.bom.tmp"
	cat "$f" >>"$f.bom.tmp"
	mv "$f.bom.tmp" "$f"
}

prepare_desktop() {
	local desktop="$ROOT/desktop"
	[ -f "$desktop/go.mod" ] || die "desktop/ 不存在，无法构建桌面端"

	if ! is_windows && [ "$WITH_DESKTOP" != 1 ]; then
		warn "非 Windows 环境：跳过桌面端 Windows exe 构建（需要时用 --with-desktop 或在 Git Bash 下运行）"
		return 0
	fi
	command -v wails3 >/dev/null 2>&1 ||
		die "未找到 wails3；先 go install github.com/wailsapp/wails/v3/cmd/wails3@latest"

	# //go:embed all:frontend/dist 要求目录存在，否则生成 bindings 会失败。
	mkdir -p "$desktop/frontend/dist"
	local placeholder="$desktop/frontend/dist/index.html"
	if [ ! -f "$placeholder" ]; then
		printf '<!doctype html><html><body></body></html>' >"$placeholder"
	fi

	(
		cd "$desktop"
		printf '==> desktop: generate bindings\n'
		wails3 generate bindings -clean=true -time-type=Date

		# Windows 桌面端 exe 是安装包里的图形入口：wails3 build 会顺带构建前端。
		printf '==> desktop: windows amd64 build\n'
		wails3 build

		# arm64 安装包需要对应架构的 exe；Windows 后端不依赖 CGO，可直接交叉编译。
		printf '==> desktop: windows arm64 build\n'
		wails3 generate syso -arch arm64 -icon build/windows/icon.ico \
			-manifest build/windows/wails.exe.manifest -info build/windows/info.json \
			-out wails_windows_arm64.syso
		CGO_ENABLED=0 GOOS=windows GOARCH=arm64 \
			go build -tags production -trimpath -buildvcs=false \
			-ldflags="-w -s -H windowsgui" -o "bin/fast-https-desktop-arm64.exe" .
		rm -f wails_windows_arm64.syso

		printf '==> desktop: windows builds done\n'
	)
}

resolve_makensis() {
	local hint=$1 c
	if [ -n "$hint" ]; then
		[ -x "$hint" ] || die "makensis 不可执行：$hint"
		printf '%s' "$hint"
		return 0
	fi
	for c in \
		/usr/bin/makensis \
		/usr/local/bin/makensis \
		"D:/NSIS/makensis.exe"; do
		if [ -n "$c" ] && [ -x "$c" ]; then
			printf '%s' "$c"
			return 0
		fi
	done
	# Git Bash 下的 Program Files 路径；带括号的 ProgramFiles(x86) 不能直接
	# 用 ${...} 展开，改从 printenv 取。
	local pf pf86
	pf=${ProgramFiles:-}
	pf86=$(printenv 'ProgramFiles(x86)' 2>/dev/null || true)
	for c in "$pf/NSIS/makensis.exe" "$pf86/NSIS/makensis.exe"; do
		if [ -n "$c" ] && [ -x "$c" ]; then
			printf '%s' "$c"
			return 0
		fi
	done
	if command -v makensis >/dev/null 2>&1; then
		command -v makensis
		return 0
	fi
	die "makensis 未找到；用 --makensis <path> 指定，或用 --no-installer 跳过"
}

# Git Bash 下 makensis 需要 Windows 路径；原生 Linux 保持 POSIX 路径。
native_path() {
	if command -v cygpath >/dev/null 2>&1; then
		cygpath -w "$1"
	else
		printf '%s' "$1"
	fi
}

# 1. desktop 前端 & goreleaser（主程序与 Monitor 的压缩包、Linux 包）
if [ "$SKIP_BUILD" != 1 ]; then
	prepare_desktop

	command -v goreleaser >/dev/null 2>&1 || die "未找到 goreleaser"
	gr_args=(release -f "$GR_CONFIG" --clean)
	if [ "$RELEASE" != 1 ]; then
		gr_args+=(--snapshot)
	fi
	# 本地打包不发布：--publish 才真正上传，需要 GITHUB_TOKEN
	if [ "$PUBLISH" != 1 ]; then
		gr_args+=(--skip=publish)
	fi

	clear_dist_temp "$ROOT/dist"
	printf '==> goreleaser %s\n' "${gr_args[*]}"
	goreleaser "${gr_args[@]}"
fi

metadata="$ROOT/dist/metadata.json"
[ -f "$metadata" ] || die "dist/metadata.json 不存在；先跑一次构建（去掉 --skip-build）"
version=$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$metadata" | head -n 1 | sed 's/.*"\([^"]*\)"$/\1/')
[ -n "$version" ] || die "无法从 $metadata 解析 version"
version4=$(to_version4 "$version")
printf '==> version %s (%s)\n' "$version" "$version4"

# 2. NSIS installers（逐个 Windows zip 生成）
if [ "$NO_INSTALLER" != 1 ]; then
	makensis=$(resolve_makensis "$MAKENSIS")
	shopt -s nullglob
	zips=("$ROOT"/dist/fast-https_Windows_*.zip)
	shopt -u nullglob
	[ ${#zips[@]} -gt 0 ] || die "dist 里没有 Windows zip，无法生成安装包"

	nsi="$ROOT/build/installer/windows/fast-https.nsi"
	ensure_bom "$nsi"
	# 自绘向导脚本：NSIS 侧同样要求 UTF-8 BOM
	ensure_bom "$ROOT/build/installer/windows/wizard.ps1"

	for zip in "${zips[@]}"; do
		arch=unknown
		case "$(basename "$zip")" in
		*x86_64*) arch=amd64 ;;
		*arm64*) arch=arm64 ;;
		esac

		stage="$ROOT/dist/.stage/$arch"
		rm -rf "$stage"
		mkdir -p "$stage"
		unzip -q "$zip" -d "$stage"

		# 桌面管理端 exe 是安装包里的图形入口；没有时安装包不含桌面包，
		# NSIS 的快捷方式会退化为安装目录，不会创建死链。
		if [ "$arch" = arm64 ]; then
			desktop_exe="$ROOT/desktop/bin/fast-https-desktop-arm64.exe"
		else
			desktop_exe="$ROOT/desktop/bin/fast-https-desktop.exe"
		fi
		if [ -f "$desktop_exe" ]; then
			cp -f "$desktop_exe" "$stage/fast-https-desktop.exe"
			size=$(awk -v b="$(wc -c <"$stage/fast-https-desktop.exe")" 'BEGIN{printf "%.1f", b/1048576}')
			printf '==> stage %s : 桌面管理端 %s MB\n' "$arch" "$size"
		else
			warn "未找到 $desktop_exe：该安装包不含桌面管理端，快捷方式会退化为安装目录"
		fi

		cp -f "$ROOT/LICENSE" "$stage/"

		out="$ROOT/dist/fast-https-windows-$arch-setup.exe"
		rm -f "$out"
		printf '==> makensis %s -> %s\n' "$arch" "$(basename "$out")"
		"$makensis" \
			"/DAPP_VERSION=$version" \
			"/DAPP_VERSION4=$version4" \
			"/DAPP_ARCH=$arch" \
			"/DSTAGE_DIR=$(native_path "$stage")" \
			"/DOUT_FILE=$(native_path "$out")" \
			"$(native_path "$nsi")"
	done
	rm -rf "$ROOT/dist/.stage"
fi

# 3. keep only artifacts in dist
keep_re='^(fast-https_.*\.(zip|tar\.gz|deb|rpm|apk)|fast-https-.*-setup\.exe|fast-https-desktop_.*\.(deb|rpm|apk)|checksums\.txt)$'
shopt -s nullglob dotglob
for p in "$ROOT"/dist/*; do
	name=$(basename "$p")
	if [[ $name =~ $keep_re ]]; then continue; fi
	rm -rf -- "$p"
done
shopt -u nullglob dotglob

printf '\n==> dist:\n'
lines=()
for f in "$ROOT"/dist/*; do
	[ -f "$f" ] || continue
	lines+=("$(awk -v n="$(basename "$f")" -v b="$(wc -c <"$f")" \
		'BEGIN { printf "  %-56s %8.1f MB", n, b/1048576 }')")
done
if [ ${#lines[@]} -gt 0 ]; then
	printf '%s\n' "${lines[@]}" | sort -k1,1
fi
