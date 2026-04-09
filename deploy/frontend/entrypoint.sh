#!/bin/sh
set -e

# =============================================================================
# Sub2API Frontend Entrypoint
# =============================================================================
# Generates nginx.conf from template using environment variables,
# then starts Nginx.
#
# Key environment variables:
#   BACKEND_SERVERS  - Backend upstream servers (required for multi-machine)
#                      Format: "10.0.1.10:8080,10.0.1.11:8080,10.0.1.12:8080"
#                      Default: "backend-1:8080" (for Docker Compose internal network)
# =============================================================================

TEMPLATE="/etc/nginx/nginx.conf.template"
OUTPUT="/etc/nginx/nginx.conf"

# If template exists, generate nginx.conf from it
if [ -f "$TEMPLATE" ]; then
    # Build upstream server list from BACKEND_SERVERS env var
    BACKEND_SERVERS="${BACKEND_SERVERS:-backend-1:8080}"

    # Convert comma-separated list to nginx upstream format
    # "10.0.1.10:8080,10.0.1.11:8080" => "server 10.0.1.10:8080;\n    server 10.0.1.11:8080;"
    UPSTREAM_SERVERS=""
    IFS=','
    for server in $BACKEND_SERVERS; do
        server=$(echo "$server" | xargs)  # trim whitespace
        if [ -n "$server" ]; then
            if [ -n "$UPSTREAM_SERVERS" ]; then
                UPSTREAM_SERVERS="${UPSTREAM_SERVERS}
        server ${server};"
            else
                UPSTREAM_SERVERS="server ${server};"
            fi
        fi
    done
    unset IFS

    export UPSTREAM_SERVERS

    # Replace environment variables in template
    envsubst '${UPSTREAM_SERVERS}' < "$TEMPLATE" > "$OUTPUT"

    echo "[entrypoint] Generated nginx.conf with upstream servers:"
    echo "  ${BACKEND_SERVERS}"
else
    echo "[entrypoint] No template found, using existing nginx.conf"
fi

# Test nginx configuration
nginx -t

# Start nginx
exec nginx -g 'daemon off;'
