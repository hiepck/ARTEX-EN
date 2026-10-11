---
name: cicd-gitops-supply-chain
description: cicd-gitops-supply-chain reference material.
---

# RCE Reference: CI/CD, GitOps & Supply-Chain Surface

Part of `skills/hunt-rce/SKILL.md`. Read this ONLY when the target has CI/CD pipelines, GitOps controllers (ArgoCD/Flux/Tekton), or a GitHub/GitLab/Jenkins organization in scope.

---

## Modern Meta — Cloud-Native, CI/CD, OSS Pipeline

This is where the 2024-2026 meta lives. Bounties scale because compromise = supply-chain compromise.

**GitHub Actions** — `pull_request_target` script injection is the dominant 2024-2026 vector. Workflow runs in **base repo's privileged context** with `GITHUB_TOKEN` carrying `contents: write` / `packages: write` / `pull-requests: write`. Any of the following primitives → secret exfil + push to main + arbitrary npm/PyPI publish:
- Direct interpolation of `${{ github.event.pull_request.title }}`, `${{ github.head_ref }}`, `${{ github.event.pull_request.body }}` into `run:` shell blocks (Cilium GHSL-2024-274/275, starrocks GHSL-2024-058/059, Ceph GHSA-p433-fp4g-pc2c, harvester GHSL-2025-090, Actual GHSL-2024-326).
- `pull_request_target` + `actions/checkout` of `pull_request.head.sha` then running `npm install` / `make` / `pytest` (openlit GHSA-9jgv-x8cq-296q, ansible.platform GHSA-fwqj-x86q-prmq, tc39 proposal-amount GHSA-43vf-c68r-43mr).
- Local action checkout (`uses: ./.github/actions/setup`) under `pull_request_target` (Actual GHSL-2024-325).
- Self-hosted runner registration token leak (workflow logs, artifact upload).

Hunting: clone target, `rg pull_request_target` and check each match for untrusted-input handling. GitHub Security Lab pays directly, plus the project usually has a parallel program.

**GitLab CI** — primary vectors: `CI_JOB_TOKEN` scope abuse (project-level token reaching org packages, CVE-2023-1080 family), `.gitlab-ci.yml` injection via mergeable branch name, runner registration token leak in build logs, GitLab Pages template SSRF→RCE chain. The 2019 GitLab archive path injection (low five-figure via `git archive --output=/var/opt/gitlab/.ssh/authorized_keys`, disclosed via H1) remains a paying-pattern when target ships its own gitaly fork.

**Jenkins** — `/script` Groovy console exposed (still found on internal/legacy assets across the H1 corpus 2017-2021), agent JNLP secret leak via `/computer/<agent>/slave-agent.jnlp`, build-parameter injection where parameter value reaches a `sh` step, `/jenkins/scriptText` no-auth via Jenkins setup wizard incomplete (CVE-2018-1000861 / CVE-2024-23897 line-too-long Args4j read primitive). Run `nuclei -t http/exposures/configs/jenkins-script-console.yaml` against any subdomain enumeration.

**ArgoCD / Flux / Tekton (GitOps controllers)** — the 2025-2026 paying surface:
- **Tekton CVE-2026-40938 (NVD-verified critical)**: git resolver passes `revision` directly to `git fetch` without `--` sentinel, `--upload-pack=/usr/bin/curl` triggers binary execution on resolver pod which holds cluster-wide Secret read.
- **OpenProject CVE-2026-24685 (NVD-verified, CVSS 9.4)**: arbitrary file write via git argument injection in repository diff endpoint.
- **ArgoCD CVE-2024-31989**: unprivileged pod can connect to ArgoCD Redis on 6379 → controller privilege escalation.
- **Argo CD CMP plugin RCE** (issue #26238): plugin admin sets up unsafe `sh -c "$ARGOCD_ENV_X"` → low-priv user injects via `spec.source.plugin.env`.
- **Argo Workflows misconfig**: insecure RBAC on `argo-server` reaching cluster admin.

**Kubernetes** — admission controller and component-direct vectors:
- **CVE-2025-1974 ingress-nginx admission controller (NVD-verified, CVSS 9.8)**: untrusted attacker on pod network → RCE on controller, which by default reads all cluster Secrets. Patches in 1.11.5 / 1.12.1.
- Kubelet anonymous auth (`--anonymous-auth=true`) → `/exec`, `/run`, `/cri/exec` endpoints reachable.
- etcd direct access (no client cert required) → cluster takeover via Secret read.
- Privileged pod escape — `securityContext.privileged: true` + hostPath `/` mount + chroot.
- IRSA confusion / cross-account role assumption — STS `AssumeRoleWithWebIdentity` with attacker JWT.
- NodePort/LoadBalancer leakage — Service exposed publicly when intended internal-only.

**Cloud IAM / IMDS** — chain entrypoints from any RCE primitive:
- IMDSv1 reachable from Lambda or container with SSRF chain (`curl http://169.254.169.254/latest/meta-data/iam/security-credentials/<role>`) → AWS keys → AssumeRole.
- IMDSv2 enforced but RCE on host bypasses it (token request is local).
- GCP metadata: `curl -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token`.
- Azure: `curl -H "Metadata: true" http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/`.
- Lambda code edit via `lambda:UpdateFunctionCode` IAM permission → arbitrary code on next invoke.
- S3 public bucket → IAM key lying in `.env.bak` → Cognito user pool admin → workspace takeover.

**Supply chain** — npm/pip/RubyGems registry vectors:
- **Dependency confusion** — internal package name registered publicly with higher version (Birsan 2021 pattern; still pays in 2024-2026 against private registries with mixed-source fallback).
- **Typosquat** — `@typescript_eslinter/eslint` (Dec 2024, Socket disclosure), `npnjs.com` phishing → `eslint-config-prettier` / `@pkgr/core` / `napi-postinstall` compromise (July 2025, Snyk and ReversingLabs joint disclosure).
- **Postinstall script abuse** — `npm install` runs arbitrary code via `package.json` `scripts.postinstall` (10 typosquats March 2026, Socket disclosure).
- **npm/pip namespace squat** — register `@org/internal-utility` after `org` lapses scope.
- **Registry token leak from CI** — `NPM_TOKEN` / `PYPI_API_TOKEN` echoed in failed-build logs that public PRs can read.
- **GitHub Actions org-level package compromise** (ansible.platform GHSA-fwqj-x86q-prmq) — `packages: write` on `GITHUB_TOKEN` lets a `pull_request_target` exploit publish malicious org-scoped packages.

**OSS supply-chain hunting workflow:** `socket dev <package>` → audit recent versions for postinstall additions → diff with previous → if suspicious, file as supply-chain incident with the package's bug bounty / security contact.

### GitOps / K8s admission

Beyond the standard ArgoCD/Flux/Tekton coverage in the cloud-native section, the admission webhook surface matters.

- **CVE-2025-1974 (ingress-nginx admission controller)** — NVD-verified CVSS 9.8. Pod-network attacker reads cluster Secrets via controller RCE.
- **OPA Gatekeeper / Kyverno policy bypass** — admission webhook race conditions, policy DoS, mutating-webhook injection.
- **Tekton ResolutionRequest objects** — CVE-2026-40938 surface above; multi-tenant clusters allowing low-priv tenants to submit ResolutionRequests are vulnerable.
- **Cilium GHSL-2024-274 / 275** — eBPF-loader CI injection via PR-controlled inputs.

Hunting: any program with K8s clusters in scope. List admission webhooks, hit them from a low-priv pod with crafted input.
