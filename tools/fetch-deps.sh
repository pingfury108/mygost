#!/usr/bin/env bash
# 准备构建期嵌入的第三方依赖：
#   1. gost 主程序（各平台压缩包）
#   2. splitd 分流插件（本仓库交叉编译）
#
# 用法：./tools/fetch-deps.sh
set -euo pipefail
cd "$(dirname "$0")/.."

GOST_VERSION="3.3.1"
GOST_DEST="internal/gostbin"
SPLIT_DEST="internal/splitbin"
mkdir -p "$GOST_DEST" "$SPLIT_DEST"

# ── gost：下载官方 release（保留压缩包，嵌入后体积约 17MB/平台）──
fetch_gost() {
  local os=$1 arch=$2 out=$3 ext="tar.gz"
  [ "$os" = "windows" ] && ext="zip"
  local url="https://github.com/go-gost/gost/releases/download/v${GOST_VERSION}/gost_${GOST_VERSION}_${os}_${arch}.${ext}"
  local tmp; tmp=$(mktemp -d)
  echo "==> gost ${GOST_VERSION} ${os}/${arch}"
  if ! curl -fsSL --retry 2 --connect-timeout 15 --max-time 600 -o "$tmp/a" "$url" 2>/dev/null; then
    echo "    (代理不可用，改为直连)"
    curl -fsSL --noproxy '*' --retry 3 --connect-timeout 15 --max-time 900 -o "$tmp/a" "$url"
  fi
  mv "$tmp/a" "$GOST_DEST/$out"
  rm -rf "$tmp"
}

fetch_gost darwin amd64 gost_darwin_amd64.tar.gz
fetch_gost darwin arm64 gost_darwin_arm64.tar.gz
fetch_gost windows amd64 gost_windows_amd64.zip

# ── splitd：本仓库源码交叉编译 ──
build_splitd() {
  local os=$1 arch=$2 out=$3
  echo "==> splitd ${os}/${arch}"
  GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o "$SPLIT_DEST/$out" ./cmd/splitd
}
build_splitd darwin amd64 splitd_darwin_amd64
build_splitd darwin arm64 splitd_darwin_arm64
build_splitd windows amd64 splitd_windows_amd64.exe

echo "==> 完成"
ls -la "$GOST_DEST" "$SPLIT_DEST"
