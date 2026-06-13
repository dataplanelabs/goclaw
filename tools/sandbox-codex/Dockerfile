# syntax=docker/dockerfile:1
FROM debian:bookworm-slim
ARG TARGETARCH=amd64
ARG CODEX_CLI_VERSION=0.139.0
ARG GH_VERSION=2.65.0
ENV DEBIAN_FRONTEND=noninteractive HOME=/home/dev

RUN apt-get update && apt-get install -y --no-install-recommends \
      bash ca-certificates curl git openssh-server tmux jq ripgrep procps \
  && rm -rf /var/lib/apt/lists/*

# codex static musl binary (multi-arch: x86_64 or aarch64)
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) MUSL_ARCH=x86_64-unknown-linux-musl ;; \
      arm64) MUSL_ARCH=aarch64-unknown-linux-musl ;; \
      *) echo "unsupported arch $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/openai/codex/releases/download/rust-v${CODEX_CLI_VERSION}/codex-${MUSL_ARCH}.tar.gz" \
      | tar -xz -C /tmp; \
    mv /tmp/codex-${MUSL_ARCH} /usr/local/bin/codex; \
    chmod +x /usr/local/bin/codex; \
    /usr/local/bin/codex --version

# gh CLI (multi-arch, native deb tarball)
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) GH_ARCH=amd64 ;; \
      arm64) GH_ARCH=arm64 ;; \
      *) echo "unsupported arch $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${GH_ARCH}.tar.gz" \
      | tar -xz -C /tmp; \
    mv /tmp/gh_*/bin/gh /usr/local/bin/gh; \
    rm -rf /tmp/gh_*; \
    gh --version

# Non-root user dev (uid/gid 1000)
RUN groupadd --gid 1000 dev && \
    useradd --create-home --uid 1000 --gid 1000 --shell /bin/bash --home-dir /home/dev dev

# sshd_config (port 2222 so the container runs as uid 1000 without CAP_NET_BIND_SERVICE)
RUN mkdir -p /etc/ssh && cat > /etc/ssh/sshd_config <<'EOF'
Port 2222
ListenAddress 0.0.0.0
PermitRootLogin no
PasswordAuthentication no
PubkeyAuthentication yes
AllowUsers dev
AuthorizedKeysFile /home/dev/.ssh/authorized_keys
HostKey /home/dev/.ssh/ssh_host_ed25519_key
X11Forwarding no
AllowTcpForwarding yes
ClientAliveInterval 300
StrictModes yes
UsePAM no
EOF

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

WORKDIR /home/dev
EXPOSE 2222

# entrypoint runs as root briefly to start sshd; sshd itself drops to dev after auth.
# Running as root is required only because sshd re-execs as uid=1000 after the host-key
# privilege-separation handshake; the process tree is non-root after login.
USER root
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
