# sandbox-codex

Minimal Debian-slim SSH pod that runs codex 0.139.0 (musl) as user `dev` (uid 1000).
GoClaw drives it over SSH via the `workstation` subsystem; codex sessions persist on PVC.

## Build

```bash
# amd64 (everest cluster is amd64-only)
docker buildx build --platform linux/amd64 -t ghcr.io/dataplanelabs/goclaw-codex-sandbox:latest tools/sandbox-codex

# local test (native arch)
docker build -t sandbox-codex:test tools/sandbox-codex
```

## Local smoke-test

```bash
# 1. Create a named volume to act as PVC, generate a throw-away SSH key, start the pod.
docker volume create codex-home
ssh-keygen -t ed25519 -N "" -f /tmp/codex_test_key

docker run -d \
  -p 2222:2222 \
  -v codex-home:/home/dev \
  -v /tmp/codex_test_key.pub:/mnt/authkeys/authorized_keys:ro \
  --name cs sandbox-codex:test

# 2. Confirm sshd is up.
sleep 2
ssh -i /tmp/codex_test_key -p 2222 -o StrictHostKeyChecking=no dev@localhost 'codex --version'

# 3. Run a headless codex task.
ssh -i /tmp/codex_test_key -p 2222 -o StrictHostKeyChecking=no dev@localhost \
  'codex exec --dangerously-bypass-approvals-and-sandbox -C /home/dev "echo hello"'

# 4. Stop + restart to verify host fingerprint stability.
docker restart cs
sleep 2
ssh -i /tmp/codex_test_key -p 2222 -o StrictHostKeyChecking=no dev@localhost 'echo ok'
```

## One-time codex login (operator, in-cluster)

ChatGPT pre-flight: Settings → Security → **Allow device code login = ON**.

```bash
# Device-code flow (preferred — no callback dependency).
kubectl -n codex exec -it deploy/codex-sandbox -- su - dev -c 'codex login --device-auth'
# → prints: Visit https://auth.openai.com/codex/device  code: XXXX-YYYY
# Open URL in any browser, sign in, enter code, Allow.

# Verify.
kubectl -n codex exec -it deploy/codex-sandbox -- su - dev -c 'codex login --status'
kubectl -n codex exec -it deploy/codex-sandbox -- su - dev -c 'test -f ~/.codex/auth.json && echo auth-persisted'

# Port-forward fallback (if device-code disabled on account).
kubectl -n codex port-forward deploy/codex-sandbox 1455:1455 &
kubectl -n codex exec -it deploy/codex-sandbox -- su - dev -c 'codex login'
```

## Persistence model

All state lives on the PVC mounted at `/home/dev`:

| Path | What | Survives restart? |
|------|------|------------------|
| `/home/dev/.ssh/ssh_host_ed25519_key` | SSH host key | Yes — generated once by entrypoint |
| `/home/dev/.ssh/authorized_keys` | GoClaw public key | Yes — refreshed from Secret on each start |
| `/home/dev/.codex/auth.json` | Codex OAuth tokens | Yes — never re-login after initial setup |
| `/home/dev/.codex/sessions/**/*.jsonl` | Codex session history | Yes — `codex exec resume --last` picks up |
| `/home/dev/repos/` | Checked-out repositories | Yes |

Host key stability means GoClaw's pinned `knownHostsFingerprint` keeps matching after pod restarts — no TOFU re-prompt.

## GoClaw wiring

1. Register workstation (admin RPC `workstations.create`):
   - `backendType: "ssh"`, `workstationKey: "codex-sandbox"`
   - `metadata.host`: `codex-sandbox.codex.svc.cluster.local`, `port: 22`, `user: "dev"`
   - `metadata.privateKey`: PEM of the GoClaw→pod private key
2. First exec triggers TOFU; grep gateway logs for `workstation.ssh_host_key_tofu fingerprint=SHA256:…` then `workstations.update` to pin it.
3. Add allowlist entries (`workstations.permAdd`): `codex`, `git`, `gh`, `bash`, `sh`, `cat`, `ls`.
4. Link the coder agent (`workstations.linkAgent`).
5. Agent calls `codex_remote {prompt: "…", repo: "/home/dev/repos/<name>"}`.
