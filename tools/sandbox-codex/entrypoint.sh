#!/usr/bin/env bash
# Idempotent entrypoint: runs as root, starts sshd as uid 0 which drops to dev after auth.
# PVC at /home/dev persists host key, authorized_keys, and codex state across restarts.
set -e

DEV_HOME=/home/dev
SSH_DIR="${DEV_HOME}/.ssh"
HOST_KEY="${SSH_DIR}/ssh_host_ed25519_key"
AUTH_KEYS_SRC="/mnt/authkeys/authorized_keys"
AUTH_KEYS_DST="${SSH_DIR}/authorized_keys"

# Ensure .ssh dir exists with correct owner/perms.
mkdir -p "${SSH_DIR}"
chown dev:dev "${DEV_HOME}" "${SSH_DIR}"
chmod 700 "${SSH_DIR}"

# Generate host key once; survives pod restarts via PVC (stable fingerprint for TOFU pin).
if [ ! -f "${HOST_KEY}" ]; then
    ssh-keygen -t ed25519 -N "" -f "${HOST_KEY}"
fi
chown dev:dev "${HOST_KEY}" "${HOST_KEY}.pub"
chmod 600 "${HOST_KEY}"

# Copy authorized_keys from projected Secret → PVC so sshd can read it.
if [ -f "${AUTH_KEYS_SRC}" ]; then
    cp "${AUTH_KEYS_SRC}" "${AUTH_KEYS_DST}"
    chown dev:dev "${AUTH_KEYS_DST}"
    chmod 600 "${AUTH_KEYS_DST}"
else
    echo "WARNING: ${AUTH_KEYS_SRC} not found; SSH login will fail until Secret is mounted" >&2
fi

# sshd -D: run in foreground; -e: log to stderr (picked up by kubectl logs).
exec /usr/sbin/sshd -D -e
