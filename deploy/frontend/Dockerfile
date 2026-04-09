# =============================================================================
# Sub2API Frontend Dockerfile (Cluster Mode)
# =============================================================================
# Builds the Vue 3 frontend and serves it via Nginx.
# API requests are reverse-proxied to the backend cluster.
#
# Build context: project root (../..)
# Usage:
#   docker build -f deploy/frontend/Dockerfile -t sub2api-frontend .
#
# Runtime environment variables:
#   BACKEND_SERVERS - Comma-separated backend addresses (default: backend-1:8080)
#                     Example: "10.0.1.10:8080,10.0.1.11:8080"
# =============================================================================

ARG NODE_IMAGE=node:24-alpine
ARG NGINX_IMAGE=nginx:1.27-alpine

# -----------------------------------------------------------------------------
# Stage 1: Frontend Builder
# -----------------------------------------------------------------------------
FROM ${NODE_IMAGE} AS frontend-builder

WORKDIR /app/frontend

# Install pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# Install dependencies first (better caching)
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Copy frontend source and build
COPY frontend/ ./
RUN pnpm run build

# The build output goes to ../backend/internal/web/dist (relative to frontend/)
# which is /app/backend/internal/web/dist in the container

# -----------------------------------------------------------------------------
# Stage 2: Nginx Runtime
# -----------------------------------------------------------------------------
FROM ${NGINX_IMAGE}

LABEL maintainer="Wei-Shaw <github.com/Wei-Shaw>"
LABEL description="Sub2API Frontend - Nginx + Vue 3 SPA (Cluster Mode)"

# Remove default nginx config
RUN rm -f /etc/nginx/conf.d/default.conf

# Copy nginx config (static fallback) and template (for dynamic upstream)
COPY deploy/frontend/nginx.conf /etc/nginx/nginx.conf
COPY deploy/frontend/nginx.conf.template /etc/nginx/nginx.conf.template

# Copy entrypoint script (generates nginx.conf from template + env vars)
COPY deploy/frontend/entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Copy frontend dist from builder
COPY --from=frontend-builder /app/backend/internal/web/dist /usr/share/nginx/html

# Expose HTTP port
EXPOSE 80

# Health check: verify Nginx is serving the frontend
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -T 3 -O /dev/null http://localhost/ || exit 1

ENTRYPOINT ["/docker-entrypoint.sh"]
