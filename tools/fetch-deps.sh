#!/usr/bin/env bash
# 交叉编译并放置各平台的 gost-geo-plugin，供 //go:embed 打包进 app。
#
# 用法：./tools/fetch-deps.sh
# 升级插件：改下面的 VERSION 后重跑本脚本，再重新构建。
set -euo pipefail
cd "$(dirname "$0")/.."

# 插件上游没有发布 tag，这里钉死到具体提交（伪版本），保证构建可复现。
VERSION="v0.0.0-20260317224525-441fb65d1129"
PKG="github.com/go-gost/gost-geo-plugin@${VERSION}"
DEST="internal/geobin"
mkdir -p "$DEST"

# go install 不支持 GOBIN + 交叉编译，改用临时模块 go build
build() {
  local os=$1 arch=$2 out=$3
  echo "==> $os/$arch → $DEST/$out"
  local tmp
  tmp=$(mktemp -d)
  (
    cd "$tmp"
    go mod init geofetch >/dev/null 2>&1
    GOFLAGS=-mod=mod go get "$PKG" >/dev/null 2>&1
    GOOS=$os GOARCH=$arch go build -trimpath -o "$OLDPWD/$DEST/$out" \
      github.com/go-gost/gost-geo-plugin
  )
  rm -rf "$tmp"
  chmod +x "$DEST/$out"
}

build darwin amd64 gost-geo-plugin_darwin_amd64
build darwin arm64 gost-geo-plugin_darwin_arm64
build windows amd64 gost-geo-plugin_windows_amd64.exe

# ── gost 主程序：从官方 release 下载并解出二进制（与 DefaultVersion 保持一致）──
GOST_VERSION="3.3.1"
GOST_DEST="internal/gostbin"
mkdir -p "$GOST_DEST"

fetch_gost() {
  local os=$1 arch=$2 out=$3 ext="tar.gz"
  [ "$os" = "windows" ] && ext="zip"
  local url="https://github.com/go-gost/gost/releases/download/v${GOST_VERSION}/gost_${GOST_VERSION}_${os}_${arch}.${ext}"
  local tmp
  tmp=$(mktemp -d)
  echo "==> gost ${GOST_VERSION} ${os}/${arch}"
  # 先按环境代理下载，失败再直连（env 里的代理可能没在跑）
  if ! curl -fsSL --retry 2 --connect-timeout 15 --max-time 600 \
      -o "$tmp/archive" "$url" 2>/dev/null; then
    echo "    (代理不可用，改为直连)"
    if ! curl -fsSL --noproxy '*' --retry 3 --connect-timeout 15 --max-time 900 \
        -o "$tmp/archive" "$url"; then
      echo "!! 下载失败：$url" >&2
      rm -rf "$tmp"
      exit 1
    fi
  fi
  # 直接保留压缩包：嵌入压缩包（约 17MB）比嵌入裸二进制（约 47MB）省得多，
  # 首次运行时解压（gostbin.extractBinary 已有实现）。
  mv "$tmp/archive" "$GOST_DEST/$out"
  rm -rf "$tmp"
}

fetch_gost darwin amd64 gost_darwin_amd64.tar.gz
fetch_gost darwin arm64 gost_darwin_arm64.tar.gz
fetch_gost windows amd64 gost_windows_amd64.zip

echo "==> 完成（geo ${VERSION} / gost ${GOST_VERSION}）"
ls -la "$DEST" "$GOST_DEST"
