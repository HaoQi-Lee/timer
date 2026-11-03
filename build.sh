#!/usr/bin/env bash
# 构建上班计时器。
#
#   ./build.sh          在当前平台构建可执行文件
#   ./build.sh mac      用 Wails CLI 打出 macOS 的 .app 包
#
# 重要：必须带 desktop,production 这两个 build tag，否则 Wails 的 CreateApp
# 会直接返回空，程序启动即退出（这是 Wails 手工构建时最容易踩的坑）。
set -euo pipefail

cd "$(dirname "$0")"

echo "==> 静态检查"
go vet ./...

echo "==> 单元测试"
go test ./...

case "${1:-native}" in
  mac)
    echo "==> 用 Wails CLI 打 macOS .app 包"
    if ! command -v wails >/dev/null 2>&1; then
      echo "未找到 wails 命令，请先执行："
      echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest"
      exit 1
    fi
    wails build -tags "desktop,production" -clean
    ;;
  *)
    echo "==> 构建可执行文件"
    # -H windowsgui：编译成图形子系统程序，双击时不会弹出黑色控制台窗口。
    # Wails 官方 CLI 在生产构建时也会自动加这个 flag。
    ldflags="-w -s"
    out="timer"
    if [ "$(go env GOOS)" = "windows" ]; then
      ldflags="$ldflags -H windowsgui"
      out="timer.exe"
    fi
    go build -tags "desktop,production" -ldflags "$ldflags" -o "$out" .
    echo "完成：$(pwd)/$out"
    ;;
esac
