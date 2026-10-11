---
name: cloud-native-and-niche-surfaces
description: cloud-native-and-niche-surfaces reference material.
---

# Cloud-Native, CI/CD, GitOps & Niche OAuth Surfaces

Part of `skills/hunt-oauth/SKILL.md`. Read this ONLY when the target is a CI/CD pipeline, GitOps controller, Kubernetes OIDC integration, ML-serving/inference platform, agentic LLM tool-use surface, or modern JS RSC/Server-Actions deployment -- not a plain web-app OAuth login.

---

## Modern Meta — Cloud-Native, CI/CD, OSS Pipeline

OAuth/OIDC/SAML/JWT primitives apply across the entire cloud-native stack — not just web apps.

- **GitHub Actions**: workflows that authenticate to cloud via OIDC use `id-token: write` permission. Hunt for `pull_request_target` workflows that compose `${{ github.event.pull_request.head.repo.full_name }}` into a script that requests OIDC tokens — script injection escalates to cloud IAM. The `actions/github-script` action mints OIDC tokens to any audience the workflow asks for; if a downstream workflow uses untrusted `audience` from PR input, it becomes a cross-org token. Salesloft Drift August 2025 incident showed *PAT in GitHub repo + AWS via OIDC + Drift OAuth tokens* as the supply-chain entrypoint that hit Cloudflare, Palo Alto, Zscaler and 757 others.

- **GitLab CI**: `CI_JOB_JWT` / `CI_JOB_TOKEN` are JWT credentials. CI_JOB_JWT supports OIDC federation to AWS / GCP / Vault. If a job's audience claim is unrestricted (`sub_pattern: project_path:*`), any project in the GitLab instance can impersonate. Hunt for cloud trust policies with `iss=https://gitlab.com` but no `sub` constraint. Also: GitLab's own CVE-2025-25291/25292 ruby-saml family means GitLab Enterprise SAML SSO with parser differentials is exploitable end-to-end (PortSwigger SAML Roulette demonstrated unauthenticated admin).

- **Jenkins**: OAuth/OIDC plugins (`oic-auth`, `azure-ad`, `github-oauth`) with relaxed `email` matching = nOAuth at scale across CI infrastructure. Jenkins script-console exposure via authenticated OIDC group claim from attacker-controlled IdP becomes RCE on the controller. Hunt every Jenkins login page for OIDC support and test with a self-hosted IdP.

- **ArgoCD / Flux**: **CVE-2025-55190 Argo CD project token leak** — a token with `projects, get` retrieves repository credentials. **CVE-2026-23990 Flux Operator** — empty OIDC `email`/`groups` claims bypass impersonation, requests run as the Flux service account. **CVE-2022-31034 Argo CD insecure entropy in PKCE/OIDC params** (still affects unpatched 2.x deployments). The pattern: GitOps controllers consume OIDC for human auth and use service tokens for machine actions; mixing these scopes loses the human's scope-restriction.

- **Kubernetes**: kubelet exposed at `:10250` with anonymous auth = RBAC bypass; if cluster uses OIDC for `kube-apiserver`, the OIDC token's `aud` claim must match `--oidc-client-id`. Many clusters set `aud=*` in trust → cross-cluster token replay. ServiceAccount projected tokens (TokenRequest API) include `kubernetes.io` audience by default; any pod that reads file-mounted projected tokens for non-K8s use cases is leaking credentials with cluster-scoped usability. **Tekton CVE-2026-40161** git resolver leaks PAT to attacker-controlled `serverURL`.

- **Cloud IAM**: AWS OIDC trust against GitHub/GitLab without `sub` constraint = cross-org token. Azure AD multi-tenant OAuth apps without `xms_edov` validation = nOAuth (CVE-2024-21632 omniauth-microsoft_graph; **CVE-2025-55241 Entra ID actor token cross-tenant Global Admin impersonation, Dirk-jan Mollema**). GCP Workload Identity Federation has analogous bugs — check that `subject_token_type` is constrained. AWS STS `AssumeRoleWithWebIdentity` requires audience binding; missing → cross-account token.

- **Supply chain (Salesloft Drift class)**: Third-party SaaS integrations use long-lived OAuth refresh tokens. UNC6395/ShinyHunters used `truffleHog` against Salesloft's GitHub repo to find Drift OAuth tokens, then exfiltrated 1.5B Salesforce records from 760 companies. Hunt your target's third-party connector inventory: each Slack app, Drift bot, HubSpot integration, Zendesk plugin is an OAuth grant your org accepted. Audit scopes (`offline_access`, `Mail.Read`, `repo`), audit revocation flows, audit logs for unusual API call rates (Salesforce Bulk API 2.0 calls under 3 minutes was the IOC).

- **Container/edge OAuth**: Cloudflare Workers OAuth Provider (the very library that hosts MCP servers) had **CVE-2025-4143 missing redirect_uri validation, CVE-2025-4144 PKCE bypass**, and **GHSA-2h78-5wx8-jccc CSRF + open redirect via state parameter**. These bugs show up in Worker code that opted into building OAuth providers without OAuth library experience. Same pattern is repeating in Bun, Deno, and edge runtime auth implementations. Hunt every edge-runtime OAuth as if it's amateur-hour.

## Modern Expansion Pack

OAuth/OIDC/SAML/JWT applies across the 2024-2026 expansion topics. The validator looks for at least 4 of 5.

1. **Container escape** — relevant via *workload identity* and *projected service-account tokens*. CVE-2024-21626 runc Leaky Vessels lets a pod escape to host; on host the kubelet TLS cert + service-account tokens become accessible, and any pod's projected OIDC token (audience `kubernetes.io`) can be exfiltrated. Combined with CVE-2024-23653 BuildKit GRPC SecurityMode missing privilege check (build-time escape), the exfiltrated tokens enable downstream cluster pivot. NVIDIA Container Toolkit (CVE-2024-0132 TOCTOU, Wiz Research) similarly grants host access from GPU pods, exposing OIDC trust to AWS/GCP for IRSA pod identity.

2. **ML serving / inference** — model registries and inference servers use OAuth tokens for tool integration and dataset retrieval. BentoML pickle deserialization (CVE-2025-27520 critical, c2an1 disclosure via Snyk) on `/summarize` endpoint accepts unauthenticated requests with `Content-Type: application/vnd.bentoml+pickle`; if the model server's OAuth token (for downstream registry, MLflow, S3) is reachable in process memory, RCE exfiltrates it. MLflow path traversal family (CVE-2024-1560/1483/1594, all via Huntr) exposes model registry credential files. TorchServe and Triton Inference Server have similar OAuth-token-on-disk patterns.

3. **Agentic LLM tool-use** — the dominant 2025-2026 OAuth attack surface. CVE-2025-68613 (LangChain `langchain-experimental` PythonREPLTool / PandasDataFrameAgent CVSS 9.8 critical) is the new attack class — indirect prompt injection in CSV/text/RAG context coerces the agent into writing exec()-able Python. The agent's OAuth tokens (for connected MCP servers, Salesforce, GitHub) are then exfiltrated via tool output. CVE-2025-49596 (Anthropic MCP Inspector unauth RCE) and CVE-2025-6514 (mcp-remote OAuth URL command injection, 558K downloads) directly compromise the OAuth client side. CVE-2026-21852 (Claude Code MCP RCE per Check Point Research) shows the malicious MCP server pattern. PolicyLayer's Supabase MCP exfiltration via tool chaining (General Analysis July 2025) and GitHub MCP prompt injection via issues (Invariant Labs May 2025) demonstrate the *cross-trust-boundary token reuse* pattern: a single OAuth token granted to the MCP server spans multiple downstream systems with no per-tool scope binding.

4. **Modern JS RSC / Server Actions** — React Server Components / Next.js App Router OAuth callback handlers run on the *backend* now. CVE-2025-66478 / CVE-2025-55182 React2Shell (CVSS 10.0, Lachlan Davidson disclosure November 29 2025, Meta Bug Bounty) means any OAuth callback handler implemented as a Server Action endpoint inherits remote code execution if React 19.0/19.1.0/19.1.1/19.2.0 packages are unpatched. Vercel maintains a separate H1 program paying for WAF bypasses against this CVE. CVE-2025-55183 (Server Actions Source Code Exposure) leaks compiled Server Function source, including OAuth client_secrets accessed from environment variables. Hunt Next.js OAuth callback at `/api/auth/callback/<provider>` and `/oauth/callback` route handlers — these are React Server Function endpoints under App Router and inherit the React deserialization bug.

5. **GitOps / K8s admission** — CVE-2025-55190 Argo CD project token leak (CVSS 7.7, GHSA-786q-9hcg-v9ff) exposes repository credentials including OAuth tokens to any project-scoped API token. CVE-2026-23990 Flux Operator empty-claims OIDC bypass (GHSA-4xh5-jcj2-ch8q) lets requests run as the operator service account. CVE-2026-40161 Tekton git resolver token exfil (GHSA-wjxp-xrpv-xpff) leaks the system Git API token (PAT, GitLab token) to attacker-controlled `serverURL`. CVE-2025-1974 ingress-nginx admission controller RCE (CVSS 9.8) — any pod-network attacker reads cluster-wide Secrets including OAuth tokens. The pattern: GitOps controllers consume OIDC for human auth and use long-lived service tokens for machine actions; mixing these scopes loses the human's scope-restriction.

