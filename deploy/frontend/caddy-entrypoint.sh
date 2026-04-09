#!/bin/sh
set -e

# =============================================================================
# Sub2API Frontend Entrypoint (Caddy)
# =============================================================================
# Generates Caddyfile from template using environment variables,
# then starts Caddy.
#
# Key environment variables:
#   BACKEND_SERVERS  - Backend servers (required)
#                      Format: "10.0.1.10:8080,10.0.1.11:8080"
#                      Default: "backend-1:8080"
#   SITE_DOMAIN      - Domain name (required for HTTPS)
#                      Use ":80" for HTTP-only / no domain
#                      Default: ":80"
#   TLS_CERT         - Path to TLS certificate file (optional)
#                      Example: "/etc/caddy/ssl/fullchain.pem"
#   TLS_KEY          - Path to TLS private key file (optional)
#                      Example: "/etc/caddy/ssl/privkey.pem"
#
# TLS modes:
#   1. Auto HTTPS:   Set SITE_DOMAIN to a real domain, leave TLS_CERT/TLS_KEY empty
#   2. Manual cert:  Set SITE_DOMAIN + TLS_CERT + TLS_KEY
#   3. HTTP only:    Set SITE_DOMAIN=":80"
# =============================================================================

TEMPLATE="/etc/caddy/Caddyfile.template"
OUTPUT="/etc/caddy/Caddyfile"

if [ -f "$TEMPLATE" ]; then
    BACKEND_SERVERS="${BACKEND_SERVERS:-backend-1:8080}"
    SITE_DOMAIN="${SITE_DOMAIN:-:80}"

    # Convert comma-separated list to Caddy upstream format
    # "10.0.1.10:8080,10.0.1.11:8080" => "10.0.1.10:8080 10.0.1.11:8080"
    UPSTREAM_SERVERS=""
    IFS=','
    for server in $BACKEND_SERVERS; do
        server=$(echo "$server" | xargs)  # trim whitespace
        if [ -n "$server" ]; then
            if [ -n "$UPSTREAM_SERVERS" ]; then
                UPSTREAM_SERVERS="${UPSTREAM_SERVERS} ${server}"
            else
                UPSTREAM_SERVERS="${server}"
            fi
        fi
    done
    unset IFS

    # Generate TLS configuration based on environment variables
    TLS_CONFIG=""
    if [ -n "$TLS_CERT" ] && [ -n "$TLS_KEY" ]; then
        # Manual certificate mode: use provided cert and key files
        if [ ! -f "$TLS_CERT" ]; then
            echo "[entrypoint] ERROR: TLS_CERT file not found: $TLS_CERT"
            exit 1
        fi
        if [ ! -f "$TLS_KEY" ]; then
            echo "[entrypoint] ERROR: TLS_KEY file not found: $TLS_KEY"
            exit 1
        fi
        TLS_CONFIG="tls ${TLS_CERT} ${TLS_KEY}"
        echo "[entrypoint] TLS mode: manual certificate"
        echo "  Cert: ${TLS_CERT}"
        echo "  Key:  ${TLS_KEY}"
    elif [ "$SITE_DOMAIN" = ":80" ] || [ "$SITE_DOMAIN" = "localhost" ]; then
        # HTTP-only mode: no TLS
        TLS_CONFIG=""
        echo "[entrypoint] TLS mode: disabled (HTTP only)"
    else
        # Auto HTTPS mode: Caddy will use Let's Encrypt / ZeroSSL
        TLS_CONFIG=""
        echo "[entrypoint] TLS mode: automatic (Let's Encrypt / ZeroSSL)"
    fi

    export UPSTREAM_SERVERS
    export SITE_DOMAIN
    export TLS_CONFIG

    # Replace environment variables in template
    envsubst '${UPSTREAM_SERVERS} ${SITE_DOMAIN} ${TLS_CONFIG}' < "$TEMPLATE" > "$OUTPUT"

    echo "[entrypoint] Generated Caddyfile:"
    echo "  Domain:   ${SITE_DOMAIN}"
    echo "  Backends: ${BACKEND_SERVERS}"
else
    echo "[entrypoint] No template found, using existing Caddyfile"
fi

# Validate config
caddy validate --config "$OUTPUT"

# Start Caddy
exec caddy run --config "$OUTPUT"
