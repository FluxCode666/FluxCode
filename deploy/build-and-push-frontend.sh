#!/usr/bin/env bash
# =============================================================================
# Sub2API - 手动构建并推送前端镜像到 GHCR
# =============================================================================
# 使用 docker buildx 构建多架构前端镜像（Vue 3 + Nginx），并推送到 GitHub Container Registry。
#
# 用法:
#   ./deploy/build-and-push-frontend.sh 1.0.0            # 构建并推送
#   ./deploy/build-and-push-frontend.sh 1.0.0 --no-push  # 仅构建，不推送
#   ./deploy/build-and-push-frontend.sh 1.0.0 --local    # 仅构建本机架构，不推送
#
# 前提条件:
#   1. 已通过 `docker login ghcr.io` 登录 GHCR
#   2. 已安装并启用 Docker Buildx
#
# 环境变量:
#   GHCR_OWNER    - GHCR 仓库所有者 (默认: 从 git remote 自动检测)
#   PLATFORMS     - 目标平台 (默认: linux/amd64,linux/arm64)
# =============================================================================

set -euo pipefail

# ---- 颜色输出 ----
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# ---- 路径 ----
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ---- 参数解析 ----
VERSION=""
NO_PUSH=false
LOCAL_ONLY=false

for arg in "$@"; do
    case "$arg" in
        --no-push)  NO_PUSH=true ;;
        --local)    LOCAL_ONLY=true; NO_PUSH=true ;;
        --help|-h)
            head -21 "$0" | tail -15
            exit 0
            ;;
        *)
            if [[ -z "$VERSION" && "$arg" =~ ^[0-9] ]]; then
                VERSION="$arg"
            else
                error "未知参数: $arg"
                exit 1
            fi
            ;;
    esac
done

# ---- 版本号（必填） ----
if [[ -z "$VERSION" ]]; then
    error "请指定版本号，例如: $0 1.0.0"
    exit 1
fi

# ---- GHCR 配置 ----
if [[ -z "${GHCR_OWNER:-}" ]]; then
    GHCR_OWNER=$(git -C "$REPO_ROOT" remote get-url origin 2>/dev/null \
        | sed -E 's#.*[:/]([^/]+)/[^/]+(.git)?$#\1#' \
        | tr '[:upper:]' '[:lower:]')
    if [[ -z "$GHCR_OWNER" ]]; then
        error "无法从 git remote 检测仓库所有者，请设置 GHCR_OWNER 环境变量"
        exit 1
    fi
fi
GHCR_OWNER=$(echo "$GHCR_OWNER" | tr '[:upper:]' '[:lower:]')

IMAGE_NAME="ghcr.io/${GHCR_OWNER}/fluxcode-frontend"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"

# ---- 打印构建信息 ----
echo ""
echo "============================================"
echo "  Sub2API 前端镜像构建"
echo "============================================"
info "镜像:     ${IMAGE_NAME}"
info "版本:     ${VERSION}"
info "平台:     ${LOCAL_ONLY:+$(docker info --format '{{.OSType}}/{{.Architecture}}' 2>/dev/null || echo 'native')}${LOCAL_ONLY:-${PLATFORMS}}"
info "推送:     $([[ "$NO_PUSH" == "true" ]] && echo "否" || echo "是")"
echo "============================================"
echo ""

# ---- 检查 Docker 登录状态（推送时） ----
if [[ "$NO_PUSH" != "true" ]]; then
    if ! docker manifest inspect ghcr.io/library/alpine:3.21 &>/dev/null 2>&1; then
        warn "请确保已通过 'docker login ghcr.io' 登录"
    fi
fi

# ---- 确保 buildx builder 存在 ----
BUILDER_NAME="sub2api-builder"
if ! docker buildx inspect "$BUILDER_NAME" &>/dev/null; then
    info "创建 buildx builder: ${BUILDER_NAME}"
    docker buildx create --name "$BUILDER_NAME" --use --bootstrap
else
    docker buildx use "$BUILDER_NAME"
fi

# ---- 构建 ----
BUILD_ARGS=(
    --file "${REPO_ROOT}/deploy/frontend/Dockerfile"
    --tag "${IMAGE_NAME}:${VERSION}"
    --tag "${IMAGE_NAME}:latest"
)

if [[ "$LOCAL_ONLY" == "true" ]]; then
    info "构建本机架构镜像..."
    docker buildx build "${BUILD_ARGS[@]}" --load "${REPO_ROOT}"
elif [[ "$NO_PUSH" == "true" ]]; then
    info "构建多架构镜像（不推送）..."
    docker buildx build "${BUILD_ARGS[@]}" --platform "${PLATFORMS}" "${REPO_ROOT}"
else
    info "构建多架构镜像并推送到 GHCR..."
    docker buildx build "${BUILD_ARGS[@]}" --platform "${PLATFORMS}" --push "${REPO_ROOT}"
fi

# ---- 完成 ----
echo ""
echo "============================================"
if [[ "$NO_PUSH" == "true" ]]; then
    ok "前端镜像构建完成！"
    if [[ "$LOCAL_ONLY" == "true" ]]; then
        info "本地镜像: ${IMAGE_NAME}:${VERSION}"
    fi
else
    ok "前端镜像构建并推送完成！"
    info "拉取命令: docker pull ${IMAGE_NAME}:${VERSION}"
fi
echo "============================================"
