#!/usr/bin/env bash
# =========================================================================
# Let's Encrypt 证书申请 & 部署脚本
# 用法: sudo ./setup-cert.sh <域名> [域名2] [域名3] ...
# 示例:
#   sudo ./setup-cert.sh oss.flux-code.cc
#   sudo ./setup-cert.sh oss.flux-code.cc oss-console.flux-code.cc
# =========================================================================
set -euo pipefail

CERT_BASE_DIR="/etc/nginx/certs"
WEBROOT="/var/www/html"

# ---- 参数校验 ----
if [ $# -lt 1 ]; then
    echo "用法: $0 <域名> [域名2] [域名3] ..."
    echo "示例: $0 oss.flux-code.cc oss-console.flux-code.cc"
    exit 1
fi

# ---- 检查 root 权限 ----
if [ "$(id -u)" -ne 0 ]; then
    echo "错误: 请使用 sudo 运行此脚本"
    exit 1
fi

# ---- 检查 certbot ----
if ! command -v certbot &>/dev/null; then
    echo "错误: certbot 未安装，请先安装: apt install certbot 或 yum install certbot"
    exit 1
fi

# ---- 构建 certbot -d 参数 ----
DOMAIN_ARGS=""
PRIMARY_DOMAIN="$1"
for DOMAIN in "$@"; do
    DOMAIN_ARGS="$DOMAIN_ARGS -d $DOMAIN"
done

echo "========================================"
echo "  申请证书: $*"
echo "  主域名:   $PRIMARY_DOMAIN"
echo "  Webroot:  $WEBROOT"
echo "========================================"

# ---- 确保 webroot 目录存在 ----
mkdir -p "$WEBROOT/.well-known/acme-challenge"

# ---- 申请证书 ----
certbot certonly \
    --webroot \
    -w "$WEBROOT" \
    $DOMAIN_ARGS \
    --non-interactive \
    --agree-tos \
    --register-unsafely-without-email \
    --keep-until-expiring

LETSENCRYPT_DIR="/etc/letsencrypt/live/$PRIMARY_DOMAIN"

if [ ! -d "$LETSENCRYPT_DIR" ]; then
    echo "错误: 证书目录 $LETSENCRYPT_DIR 不存在，申请可能失败"
    exit 1
fi

# ---- 为每个域名部署证书 ----
for DOMAIN in "$@"; do
    TARGET_DIR="$CERT_BASE_DIR/$DOMAIN"
    echo "部署证书到 $TARGET_DIR ..."

    mkdir -p "$TARGET_DIR"
    cp -L "$LETSENCRYPT_DIR/fullchain.pem" "$TARGET_DIR/fullchain.pem"
    cp -L "$LETSENCRYPT_DIR/privkey.pem"   "$TARGET_DIR/privkey.pem"
    cp -L "$LETSENCRYPT_DIR/chain.pem"     "$TARGET_DIR/chain.pem"

    chmod 600 "$TARGET_DIR/privkey.pem"
    chmod 644 "$TARGET_DIR/fullchain.pem" "$TARGET_DIR/chain.pem"

    echo "  ✓ $DOMAIN 完成"
done

echo ""
echo "========================================"
echo "  证书部署完成！请手动验证并重载 nginx:"
echo "    sudo nginx -t && sudo systemctl reload nginx"
echo "========================================"

