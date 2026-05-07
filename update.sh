#!/usr/bin/env bash
# Dujiao-Next API 一键更新脚本（适配宝塔 Go 项目管理）
#
# 用法：
#   bash update.sh                    # 默认拉 chore/local-init
#   bash update.sh main               # 指定分支
#   BRANCH=main bash update.sh        # 也可用环境变量
#
# 行为：
#   1. 标记 git 安全目录、确保分支存在
#   2. 备份 SQLite data/ 目录（若存在）
#   3. git fetch + ff-only pull
#   4. go mod tidy + go build 到 .new 文件，再原子替换
#   5. 将产物 chown 给 RUN_USER（默认 www）
#   6. 通过 baota 守护或 pkill 触发重启

set -euo pipefail

# ===== 可调参数 =====
BRANCH="${1:-${BRANCH:-chore/local-init}}"
RUN_USER="${RUN_USER:-www}"
RUN_GROUP="${RUN_GROUP:-www}"
BIN_NAME="${BIN_NAME:-dujiao-api}"
BUILD_PKG="${BUILD_PKG:-./cmd/server}"
KEEP_BACKUPS="${KEEP_BACKUPS:-5}"   # data/ 备份保留数量
# ====================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

log()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!! \033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mxx \033[0m %s\n' "$*" >&2; exit 1; }

# ---------- 0. 前置检查 ----------
command -v git >/dev/null 2>&1 || die "未找到 git"
command -v go  >/dev/null 2>&1 || die "未找到 go，请在宝塔『软件商店 → Go 版本管理』确认已启用"

log "项目目录: $SCRIPT_DIR"
log "目标分支: $BRANCH"
log "Go 版本:  $(go version)"

# ---------- 1. git 安全目录 ----------
if ! git -C "$SCRIPT_DIR" rev-parse --git-dir >/dev/null 2>&1; then
  log "标记 git 安全目录"
  git config --global --add safe.directory "$SCRIPT_DIR"
fi

# ---------- 2. 备份 SQLite ----------
if [[ -d data ]]; then
  TS="$(date +%Y%m%d-%H%M%S)"
  BACKUP="data.bak.$TS"
  log "备份 data/ -> $BACKUP"
  cp -a data "$BACKUP"
  # 保留最近 N 份
  ls -1dt data.bak.* 2>/dev/null | tail -n +$((KEEP_BACKUPS + 1)) | xargs -r rm -rf
else
  warn "未发现 data/ 目录，跳过备份（PostgreSQL 用户请自行 pg_dump）"
fi

# ---------- 3. 拉取代码 ----------
log "fetch origin $BRANCH"
# 兼容 single-branch 克隆：先把分支加入 fetch 列表
git remote set-branches --add origin "$BRANCH" >/dev/null 2>&1 || true
git fetch origin "$BRANCH"
git fetch --tags origin >/dev/null 2>&1 || warn "拉取 tags 失败，将使用提交号作为版本号"

# 工作区干净检查
if [[ -n "$(git status --porcelain)" ]]; then
  warn "工作区存在未提交改动，已自动 stash"
  git stash push -u -m "update.sh-autostash-$(date +%s)" >/dev/null
fi

log "checkout & ff-only pull"
git checkout -B "$BRANCH" "origin/$BRANCH"
git pull --ff-only origin "$BRANCH"

CURRENT_COMMIT="$(git rev-parse --short HEAD)"
log "当前提交: $CURRENT_COMMIT"

# ---------- 4. 编译 ----------
log "go mod tidy"
go mod tidy

# 计算注入版本号：
#   1) APP_VERSION 环境变量可手动覆盖
#   2) 优先使用 git describe --tags，例如 v1.3.5 或 v1.3.5-1-gf811533
#      后台更新检测只解析主版本号部分，因此 v1.3.5-1-gxxxx 会按 v1.3.5 比较
#   3) 没有 tag 时回退到短提交号
APP_VERSION="${APP_VERSION:-}"
if [[ -z "$APP_VERSION" ]]; then
  APP_VERSION="$(git describe --tags --always --dirty 2>/dev/null || git rev-parse --short HEAD)"
fi
LDFLAGS="-s -w -X github.com/dujiao-next/internal/version.Version=${APP_VERSION}"

log "注入版本号: $APP_VERSION"
log "go build -> ${BIN_NAME}.new"
CGO_ENABLED="${CGO_ENABLED:-1}" go build -trimpath -ldflags "$LDFLAGS" -o "${BIN_NAME}.new" "$BUILD_PKG"

log "原子替换二进制"
mv -f "${BIN_NAME}.new" "$BIN_NAME"
chmod +x "$BIN_NAME"

# ---------- 5. 修正属主 ----------
if id "$RUN_USER" >/dev/null 2>&1; then
  log "chown -R ${RUN_USER}:${RUN_GROUP}"
  chown -R "${RUN_USER}:${RUN_GROUP}" "$SCRIPT_DIR"
else
  warn "用户 $RUN_USER 不存在，跳过 chown"
fi

# ---------- 6. 触发重启 ----------
log "尝试重启服务"
RESTARTED=0

# 6.1 systemd（如果你单独配过）
if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files | grep -q "^${BIN_NAME}\.service"; then
  systemctl restart "${BIN_NAME}.service" && RESTARTED=1
  log "已通过 systemd 重启 ${BIN_NAME}.service"
fi

# 6.2 宝塔 Go 项目守护：杀掉旧进程，由保活自动拉起
if [[ "$RESTARTED" -eq 0 ]]; then
  if pgrep -f "$SCRIPT_DIR/$BIN_NAME" >/dev/null 2>&1; then
    pkill -f "$SCRIPT_DIR/$BIN_NAME" || true
    sleep 1
    log "已 kill 旧进程，等待宝塔守护拉起新进程"
    RESTARTED=1
  fi
fi

if [[ "$RESTARTED" -eq 0 ]]; then
  warn "未检测到运行中的 $BIN_NAME 进程，请到『宝塔 → Go 项目』手动启动"
fi

# ---------- 7. 健康检查（可选） ----------
PORT="${HEALTH_PORT:-}"
if [[ -n "$PORT" ]]; then
  for i in 1 2 3 4 5 6 7 8 9 10; do
    if curl -fsS "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then
      log "健康检查通过 (port $PORT)"
      break
    fi
    sleep 1
    [[ $i -eq 10 ]] && warn "健康检查未通过，请查看『项目日志』"
  done
fi

log "完成 ✓ 当前提交 $CURRENT_COMMIT  版本号 $APP_VERSION"
echo
echo "查看运行版本的方式："
echo "  - 启动日志: 宝塔 → Go 项目 → 项目日志，开头会有 'Version: ${APP_VERSION}'"
[[ -n "${HEALTH_PORT:-}" ]] && echo "  - 公开接口: curl -s http://127.0.0.1:${HEALTH_PORT}/api/v1/public/site | grep app_version"
echo "  - 管理后台: GET /api/v1/admin/system/version/check (需登录)"
