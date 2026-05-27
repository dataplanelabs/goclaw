# syntax=docker/dockerfile:1

# ENABLE_EMBEDUI controls whether the web UI is built and embedded.
# Must be declared before first FROM to use in stage selector.
ARG ENABLE_EMBEDUI=false

# GWS_CLI_VERSION must be declared in global scope so it can be used in a
# stage selector below. buildx no longer supports variable expansion in
# COPY --from (see https://github.com/moby/buildkit/pull/4034).
ARG GWS_CLI_VERSION=0.1.0

# Pre-pull the gws-cli site-packages as a named stage so the downstream
# COPY --from=gws-cli can use a static stage name (no ARG in --from).
FROM ghcr.io/dataplanelabs/gws-cli:${GWS_CLI_VERSION} AS gws-cli

# ── Stage 0: Build Web UI ──
# BuildKit skips this stage entirely when ENABLE_EMBEDUI=false
# because no downstream stage in the dependency graph references it.
FROM node:22-alpine AS web-builder
RUN corepack enable && corepack prepare pnpm@10.28.2 --activate
WORKDIR /app
# Copy .npmrc first so pnpm resolves musl native bindings (needed on Alpine).
# The lockfile already includes musl entries thanks to supportedArchitectures in .npmrc.
COPY ui/web/.npmrc ui/web/package.json ui/web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY ui/web/ .
RUN pnpm build

# ── Stage selector: pick web-builder output or empty dir ──
FROM web-builder AS embedui-true
FROM busybox AS embedui-false
RUN mkdir -p /app/dist
FROM embedui-${ENABLE_EMBEDUI} AS web-dist

# ── Stage 1: Build Go ──
FROM golang:1.26-bookworm AS builder

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build args (re-declare after FROM; top-level ARG only visible in FROM lines)
ARG ENABLE_OTEL=false
ARG ENABLE_TSNET=false
ARG ENABLE_REDIS=false
ARG ENABLE_EMBEDUI=false
ARG VERSION=

# Copy web UI dist — from web-builder when ENABLE_EMBEDUI=true, empty dir otherwise.
COPY --from=web-dist /app/dist /src/internal/webui/dist

RUN set -eux; \
    if [ -z "$VERSION" ] && [ -f VERSION ]; then VERSION=$(cat VERSION); fi; \
    if [ -z "$VERSION" ]; then VERSION="dev"; fi; \
    TAGS=""; \
    if [ "$ENABLE_EMBEDUI" = "true" ]; then TAGS="embedui"; fi; \
    if [ "$ENABLE_OTEL" = "true" ]; then \
        if [ -n "$TAGS" ]; then TAGS="$TAGS,otel"; else TAGS="otel"; fi; \
    fi; \
    if [ "$ENABLE_TSNET" = "true" ]; then \
        if [ -n "$TAGS" ]; then TAGS="$TAGS,tsnet"; else TAGS="tsnet"; fi; \
    fi; \
    if [ "$ENABLE_REDIS" = "true" ]; then \
        if [ -n "$TAGS" ]; then TAGS="$TAGS,redis"; else TAGS="redis"; fi; \
    fi; \
    # nodynamic: force gen2brain/jpegxl's WASM-only path so the binary stays
    # statically linked (Alpine musl runtime has no glibc dynamic loader).
    if [ -n "$TAGS" ]; then TAGS="$TAGS,nodynamic"; else TAGS="nodynamic"; fi; \
    TAGS="-tags $TAGS"; \
    CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w -X github.com/nextlevelbuilder/goclaw/cmd.Version=${VERSION}" \
    ${TAGS} -o /out/goclaw . && \
    CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -o /out/pkg-helper ./cmd/pkg-helper

# ── Stage 2: Runtime ──
FROM python:3.12-slim-bookworm

ARG ENABLE_SANDBOX=false
ARG ENABLE_PYTHON=false
ARG ENABLE_NODE=false
ARG ENABLE_FULL_SKILLS=false
ARG ENABLE_CLAUDE_CLI=false

COPY docker/requirements-base.txt docker/requirements-skills.txt /tmp/

RUN set -eux; \
    export DEBIAN_FRONTEND=noninteractive; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates wget gosu curl gnupg; \
    ln -sf /usr/sbin/gosu /usr/local/bin/su-exec; \
    if [ "$ENABLE_SANDBOX" = "true" ]; then \
        apt-get install -y --no-install-recommends docker.io; \
    fi; \
    if [ "$ENABLE_FULL_SKILLS" = "true" ]; then \
        apt-get install -y --no-install-recommends nodejs npm pandoc poppler-utils ffmpeg libsndfile1 git \
            libreoffice \
            build-essential cmake pkg-config; \
        wget -qO- https://github.com/cli/cli/releases/download/v2.65.0/gh_2.65.0_linux_amd64.tar.gz \
            | tar -xz -C /tmp && mv /tmp/gh_*/bin/gh /usr/local/bin/gh && rm -rf /tmp/gh_*; \
        curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain 1.83.0 --profile minimal; \
        export PATH="/root/.cargo/bin:$PATH"; \
        pip3 install --no-cache-dir --break-system-packages \
            --extra-index-url https://abetlen.github.io/llama-cpp-python/whl/cpu \
            -r /tmp/requirements-base.txt -r /tmp/requirements-skills.txt; \
        /root/.cargo/bin/rustup self uninstall -y; \
        apt-get purge -y --auto-remove build-essential cmake pkg-config; \
        npm install -g --cache /tmp/npm-cache docx@^9.6.1 pptxgenjs@^4.0.1; \
        python3 -c "import boto3, docx, openpyxl, pandas, pptx, pyzipper, pypdf, pdfplumber, pdf2image, markitdown, PIL"; \
        NODE_PATH="$(npm root -g)" node -e "require.resolve('docx'); require.resolve('pptxgenjs')"; \
        command -v soffice >/dev/null; \
        command -v pandoc >/dev/null; \
        command -v pdftoppm >/dev/null; \
        command -v ffmpeg >/dev/null; \
        rm -rf /tmp/npm-cache /root/.cache; \
    else \
        if [ "$ENABLE_PYTHON" = "true" ]; then \
            pip3 install --no-cache-dir --break-system-packages -r /tmp/requirements-base.txt; \
        fi; \
        if [ "$ENABLE_NODE" = "true" ] || [ "$ENABLE_CLAUDE_CLI" = "true" ]; then \
            apt-get install -y --no-install-recommends nodejs npm; \
        fi; \
    fi; \
    if [ "$ENABLE_CLAUDE_CLI" = "true" ]; then \
        npm install -g --cache /tmp/npm-cache @anthropic-ai/claude-code@^2.1.91; \
        rm -rf /tmp/npm-cache; \
    fi; \
    rm -f /tmp/requirements-base.txt /tmp/requirements-skills.txt; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 1000 -d /app -s /bin/sh goclaw 2>/dev/null || true
WORKDIR /app

# Copy binary, migrations, and bundled skills
COPY --from=builder /out/goclaw /app/goclaw
COPY --from=builder /out/pkg-helper /app/pkg-helper
COPY --from=builder /src/migrations/ /app/migrations/
COPY --from=builder /src/skills/ /app/bundled-skills/
COPY docker-entrypoint.sh /app/docker-entrypoint.sh

COPY internal/audio/vieneu-sidecar/ /app/vieneu-sidecar/
RUN set -eux; \
    if [ "$ENABLE_FULL_SKILLS" = "true" ]; then \
        python3 -c "from vieneu import Vieneu; Vieneu()" || true; \
    else \
        rm -rf /app/vieneu-sidecar; \
    fi

# B3-01 Phase 5: bake the gws-cli Python wrapper when ENABLE_FULL_SKILLS=true.
# The image already has python3 from the full-skills layer. We copy the package
# from the published gws-cli image to /app/gws-cli/ and symlink /usr/local/bin/gws
# → `python3 -m gws_cli` shim so `secure_cli_run` can invoke `gws`.
COPY --from=gws-cli /app/site-packages /app/gws-cli/site-packages
RUN set -eux; \
    if [ "$ENABLE_FULL_SKILLS" = "true" ]; then \
        printf '#!/bin/sh\nexec python3 -c "import sys; sys.path.insert(0, \"/app/gws-cli/site-packages\"); from gws_cli.cli import main; main()" "$@"\n' > /usr/local/bin/gws; \
        chmod +x /usr/local/bin/gws; \
    else \
        rm -rf /app/gws-cli; \
    fi

# Fix Windows git clone issues:
# 1. CRLF line endings in shell scripts (Windows git adds \r)
# 2. Broken symlinks: On Windows (core.symlinks=false), git creates text files
#    or skips symlinks entirely. Skills like docx/pptx/xlsx need _shared/office
#    module in their scripts/ dir (originally symlinked as scripts/office -> ../../_shared/office).
RUN set -eux; \
    sed -i 's/\r$//' /app/docker-entrypoint.sh; \
    cd /app/bundled-skills; \
    for skill in docx pptx xlsx; do \
        if [ -d "${skill}/scripts" ] && [ ! -d "${skill}/scripts/office" ]; then \
            rm -f "${skill}/scripts/office"; \
            cp -r _shared/office "${skill}/scripts/office"; \
        fi; \
    done

RUN chmod +x /app/docker-entrypoint.sh && \
    chmod 755 /app/pkg-helper && chown root:root /app/pkg-helper

# Create data directories.
# .runtime has split ownership: root owns the dir (so pkg-helper can write apk-packages),
# while pip/npm subdirs are goclaw-owned (runtime installs by the app process).
# Symlink .claude → data volume so Claude CLI credentials persist across container recreates.
RUN mkdir -p /app/workspace /app/data/.runtime/pip /app/data/.runtime/npm-global/lib \
        /app/data/.runtime/pip-cache /app/data/.runtime/bin /app/data/.claude /app/skills \
        /app/tsnet-state /app/.goclaw \
    && ln -s /app/data/.claude /app/.claude \
    && touch /app/data/.runtime/apk-packages \
    && chown -R goclaw:goclaw /app/workspace /app/skills /app/tsnet-state /app/.goclaw \
    && chown goclaw:goclaw /app/bundled-skills /app/data \
    && chown root:goclaw /app/data/.runtime /app/data/.runtime/apk-packages \
    && chmod 0750 /app/data/.runtime \
    && chmod 0640 /app/data/.runtime/apk-packages \
    && chown -R goclaw:goclaw /app/data/.runtime/pip /app/data/.runtime/npm-global /app/data/.runtime/pip-cache /app/data/.runtime/bin /app/data/.claude \
    && chmod 0755 /app/data/.runtime/bin

# Default environment
ENV GOCLAW_CONFIG=/app/config.json \
    GOCLAW_WORKSPACE=/app/workspace \
    GOCLAW_DATA_DIR=/app/data \
    GOCLAW_SKILLS_DIR=/app/skills \
    GOCLAW_MIGRATIONS_DIR=/app/migrations \
    GOCLAW_HOST=0.0.0.0 \
    GOCLAW_PORT=18790

# Entrypoint runs as root to install persisted packages and start pkg-helper,
# then drops to goclaw user via su-exec before starting the app.

EXPOSE 18790

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:18790/health || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["serve"]
