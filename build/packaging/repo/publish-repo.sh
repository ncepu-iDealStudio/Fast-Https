#!/usr/bin/env bash
# 把 dist 里的 deb / rpm / apk 组装成可以直接静态托管的仓库树：
#
#   OUT/apt/   pool + dists/stable（Packages、Release、InRelease）
#   OUT/yum/   x86_64/、aarch64/ + repodata/（createrepo_c）
#   OUT/apk/   x86_64/、aarch64/ + APKINDEX.tar.gz（apk-tools，可选）
#   OUT/apt/fast-https.list、OUT/yum/fast-https.repo  给用户直接下发
#
# 用法：
#   publish-repo.sh dist repo-out [GPG_KEY_ID] [BASE_URL]
#
# GPG_KEY_ID 为空时跳过签名（不加签只能本地测试或配合 [trusted=yes]）；
# BASE_URL 决定生成的 .list / .repo 里的地址，例如
#   https://repo.example.com/fast-https
#
# 依赖：dpkg-scanpackages、apt-ftparchive（apt 源）；createrepo_c（yum 源）；
#       gpg（签名）；可选 apk + abuild-sign（Alpine 源）。
set -euo pipefail

DIST="${1:?usage: publish-repo.sh DIST_DIR OUT_DIR [GPG_KEY_ID] [BASE_URL]}"
OUT="${2:?usage: publish-repo.sh DIST_DIR OUT_DIR [GPG_KEY_ID] [BASE_URL]}"
KEY="${3:-}"
BASE_URL="${4:-https://repo.example.com/fast-https}"
# 由签名分支覆盖；yum 的 .repo 模板里 repo_gpgcheck 用它。
REPO_GPGCHECK=0

log() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

[ -d "$DIST" ] || die "dist 目录不存在：$DIST"
shopt -s nullglob
debs=("$DIST"/fast-https_*_linux_*.deb)
rpms=("$DIST"/fast-https_*_linux_*.rpm)
apks=("$DIST"/fast-https_*_linux_*.apk)
[ "${#debs[@]}" -gt 0 ] || die "dist 里没有 .deb：先跑 goreleaser release --snapshot（或 -Release）"

# goreleaser 的文件名形如 fast-https_<version>_linux_amd64.deb
arch_of() { sed -E 's/.*_linux_([a-z0-9]+)\.[a-z]+$/\1/' <<<"$1"; }

rm -rf "$OUT/apt" "$OUT/yum" "$OUT/apk"
mkdir -p "$OUT/apt" "$OUT/yum"

# ---------- APT ----------
if have dpkg-scanpackages && have apt-ftparchive; then
    log "APT 仓库"
    mkdir -p "$OUT/apt/pool/main/f/fast-https" "$OUT/apt/dists/stable/main"
    cp "${debs[@]}" "$OUT/apt/pool/main/f/fast-https/"

    for deb in "${debs[@]}"; do
        arch="$(arch_of "$deb")"
        case "$arch" in
            amd64|arm64) ;;
            *) warn "跳过未知 deb 架构：$deb"; continue ;;
        esac
        dir="$OUT/apt/dists/stable/main/binary-$arch"
        mkdir -p "$dir"
        (cd "$OUT/apt" &&
            dpkg-scanpackages --arch "$arch" pool /dev/null >"dists/stable/main/binary-$arch/Packages")
        gzip -9 -kf "$dir/Packages"
    done

    (cd "$OUT/apt/dists/stable" &&
        apt-ftparchive \
            -o APT::FTPArchive::Release::Origin="Fast-Https" \
            -o APT::FTPArchive::Release::Label="Fast-Https" \
            -o APT::FTPArchive::Release::Suite="stable" \
            -o APT::FTPArchive::Release::Codename="stable" \
            -o APT::FTPArchive::Release::Architectures="amd64 arm64" \
            -o APT::FTPArchive::Release::Components="main" \
            -o APT::FTPArchive::Release::Description="Fast-Https APT repository" \
            release . >Release)

    if [ -n "$KEY" ]; then
        have gpg || die "指定了 GPG_KEY_ID 但没有 gpg"
        (cd "$OUT/apt/dists/stable" &&
            gpg --batch --yes --local-user "$KEY" --clearsign -o InRelease Release &&
            gpg --batch --yes --local-user "$KEY" -abs -o Release.gpg Release)
        # 下发公钥：.asc 给 dnf 当 gpgkey，.gpg（二进制 keyring）给 apt 当 signed-by。
        gpg --batch --yes --armor --export "$KEY" >"$OUT/fast-https.asc"
        gpg --batch --yes --export "$KEY" >"$OUT/fast-https.gpg"
        log "APT 索引已签名（$KEY），公钥已导出为 fast-https.asc / fast-https.gpg"
        SIGNED_BY=" [signed-by=/etc/apt/keyrings/fast-https.gpg]"
        REPO_GPGCHECK=1
    else
        warn "未提供 GPG_KEY_ID：跳过 InRelease 签名，客户端需要 trusted=yes / repo_gpgcheck=0 才能用"
        SIGNED_BY=""
        REPO_GPGCHECK=0
    fi

    sed -e "s|@BASE_URL@|$BASE_URL|g" -e "s|@SIGNED_BY@|$SIGNED_BY|g" \
        "$(dirname "$0")/apt/fast-https.list" >"$OUT/apt/fast-https.list"
else
    die "缺少 dpkg-scanpackages / apt-ftparchive：apt 源要在 Debian 系或装了 dpkg-dev + apt-utils 的环境里生成"
fi

# ---------- YUM / DNF ----------
if [ "${#rpms[@]}" -gt 0 ]; then
    if have createrepo_c; then
        log "YUM 仓库"
        declare -A dirs=()
        for rpm in "${rpms[@]}"; do
            case "$(arch_of "$rpm")" in
                amd64) sub=x86_64 ;;
                arm64) sub=aarch64 ;;
                *) warn "跳过未知 rpm 架构：$rpm"; continue ;;
            esac
            mkdir -p "$OUT/yum/$sub"
            cp "$rpm" "$OUT/yum/$sub/"
            dirs["$sub"]=1
        done
        for sub in "${!dirs[@]}"; do
            createrepo_c --database "$OUT/yum/$sub"
            if [ -n "$KEY" ] && have gpg; then
                gpg --batch --yes --local-user "$KEY" -abs \
                    -o "$OUT/yum/$sub/repodata/repomd.xml.asc" \
                    "$OUT/yum/$sub/repodata/repomd.xml"
            fi
        done
        sed -e "s|@BASE_URL@|$BASE_URL|g" -e "s|@REPO_GPGCHECK@|$REPO_GPGCHECK|g" \
            "$(dirname "$0")/yum/fast-https.repo" >"$OUT/yum/fast-https.repo"
    else
        warn "缺少 createrepo_c：跳过 yum 源（apt install createrepo-c / dnf install createrepo_c）"
    fi
fi

# ---------- APK（Alpine，可选）----------
if [ "${#apks[@]}" -gt 0 ] && have apk; then
    log "APK 仓库"
    for apkfile in "${apks[@]}"; do
        case "$(arch_of "$apkfile")" in
            amd64) sub=x86_64 ;;
            arm64) sub=aarch64 ;;
            *) warn "跳过未知 apk 架构：$apkfile"; continue ;;
        esac
        mkdir -p "$OUT/apk/$sub"
        cp "$apkfile" "$OUT/apk/$sub/"
    done
    for sub in "$OUT"/apk/*/; do
        (cd "$sub" && apk index -o APKINDEX.tar.gz --rewrite-arch "$(basename "$sub")" ./*.apk)
        if [ -n "$KEY" ] && have abuild-sign; then
            (cd "$sub" && abuild-sign -k "$KEY" APKINDEX.tar.gz)
        fi
    done
elif [ "${#apks[@]}" -gt 0 ]; then
    warn "缺少 apk-tools：跳过 Alpine 源"
fi

# ---------- 汇总 ----------
if [ -f "$DIST/checksums.txt" ]; then
    cp "$DIST/checksums.txt" "$OUT/"
fi
(cd "$OUT" && find . -type f ! -name SHA256SUMS -exec sha256sum {} + | sort -k2 >SHA256SUMS)

log "仓库树已生成：$OUT（记得把整棵树（含 fast-https.asc/.gpg）传到 $BASE_URL）"
log "Debian/Ubuntu:"
log "  curl -fsSL $BASE_URL/fast-https.gpg | sudo tee /etc/apt/keyrings/fast-https.gpg >/dev/null"
log "  sudo cp apt/fast-https.list /etc/apt/sources.list.d/fast-https.list"
log "  sudo apt update && sudo apt install fast-https"
log "RHEL/CentOS:"
log "  sudo cp yum/fast-https.repo /etc/yum.repos.d/fast-https.repo"
log "  sudo dnf makecache && sudo dnf install fast-https"
log "Alpine:"
log "  echo '$BASE_URL/apk/\$(apk --print-arch)' >> /etc/apk/repositories && apk update && apk add fast-https"
