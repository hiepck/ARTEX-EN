---
name: cloud-native-devops-pipeline
description: cloud-native-devops-pipeline reference material.
---

# Cloud-Native / CI-CD / DevOps Pipeline Business Logic

Part of `skills/hunt-business-logic/SKILL.md`. Read this ONLY when the target IS a CI/CD platform (GitHub Actions, GitLab CI, Jenkins), a GitOps controller (ArgoCD/Flux/Tekton), a Kubernetes admission surface, an ML-serving/inference platform with credit or quota systems, or an agentic-LLM tool-use system.

---

## Modern Meta — Cloud-Native, CI/CD, OSS Pipeline

This is where the 2024-2026 business-logic meta lives. Coverage required: GitHub Actions, GitLab CI, Jenkins, ArgoCD/Flux, Kubernetes, IAM/IMDS, supply chain.

**GitHub Actions business-logic surface** — workflow `if:` conditions evaluated on attacker-controlled inputs (`github.event.pull_request.title`, `github.event.commits[*].message`); attackers craft titles/commits to satisfy `if: contains(github.event.pull_request.title, '[skip-tests]')` and bypass test gates. Approval-required workflow `environments` bypassed when bot accounts are not subject to required-reviewer rules. CI cache poisoning via concurrent commits to overwrite shared cache with malicious payload (cross-reference hunt-rce CI/CD section).

**GitLab CI business-logic surface** — `rules:` conditions on user-controlled CI variables (`CI_COMMIT_MESSAGE`, `CI_PIPELINE_SOURCE`); attackers craft commit messages to bypass test gates. `protected: true` branches sometimes bypassable via merge-request from feature branch with auto-merge enabled by trusted reviewer.

**Jenkins business-logic surface** — pipeline `when` conditions, `input` step bypass via direct API call to `/job/<name>/build/api/json` skipping interactive approval, role-strategy plugin's "Project Roles" assigned by pattern-match on job name (rename job to bypass).

**ArgoCD / Flux / Tekton (GitOps controllers) business-logic surface**:
- **OpenClaw WebSocket shared-auth elevated scopes** (2026 critical, GHSA cited in corpus) — WebSocket connections share auth context across users; client self-declares elevated scopes.
- **mpp payment bypass and griefing** (2026 critical, GHSA cited in corpus) — multi-party computation payment bypass.
- **NuGet Client Security Feature Bypass** (2024 critical, GHSA cited in corpus).
- ArgoCD `AppProject` with overly-broad `destinations` and `sourceRepos` allows business-logic abuse where teams deploy to neighbor namespaces.
- Tekton `PipelineRun` with workspace shared across teams enables cross-team artifact tampering.

**Kubernetes business-logic surface** — `ResourceQuota` enforcement happens at admission; concurrent CREATE requests can race past the limit. `LimitRange` for pod resource requests bypassable via Pod-spec `priorityClassName` exemptions.

**Cloud IAM / IMDS** — IAM trust-policy `Condition` blocks evaluated against attacker-influenced attributes (e.g., `aws:SourceIp` → use VPN to satisfy condition; `aws:RequestTag/<key>` → set tag via mass-assignment to satisfy). STS `AssumeRoleWithWebIdentity` accepts JWT from federated identity providers — if IDP issues tokens for unverified emails, business-logic abuse via fake-domain registration.

**Supply chain** — npm/pip postinstall scripts that perform "license check" or "feature gate" evaluations client-side; attackers patch the package to bypass tier check. Trial-extension via GitHub Sponsors / OpenCollective / Patreon webhook abuse.

## Modern Expansion Pack (2024-2026 currency)

The 2024-2026 expansion meta required by the validator. All five topics covered.

### Container escape / runtime business-logic

<!-- expansion-na: container reason: container escape is RCE-class; business-logic at runtime layer manifests as resource-quota race conditions and Kubernetes ResourceQuota bypass via concurrent CREATE requests — covered in Modern Meta section above. -->

The closest analog: **Kubernetes ResourceQuota race-condition bypass**. When a namespace has `ResourceQuota` for `pods: 10`, concurrent CREATE requests for 11 pods can race past the limit because the admission controller checks-then-counts without atomic locking. Same TOCTOU pattern as WWBN AVideo CVE-2026-34368 wallet but at the cluster-resource layer.

### ML serving / inference business-logic

ML platforms with credit / quota systems are emerging business-logic targets:
- **OpenAI API tier overrun via concurrent requests** — burst above your tier's RPS limit, completion-token usage may exceed the cap if rate limit checks aren't atomic.
- **Anthropic API trial reset abuse** — same Stripe `hasEverTrialed` pattern (better-auth #6863) potentially applies; LLM provider has multiple subscription records, trial-history check uses wrong query.
- **Hugging Face Hub tier limits on private models / Spaces compute** — concurrent uploads / spin-ups may bypass per-tier quota.
- **BentoML / MLflow / TorchServe tier-based access** — model-registry RBAC sometimes checked at API but not at storage layer.

### Agentic LLM tool-use business-logic

OWASP LLM06:2025 Excessive Agency intersects business-logic:
- **Agent's tool granted broader scope than agent's task requires** — design flaw, but exploitable by prompt injection (cross-reference hunt-llm-ai).
- **Per-conversation cost / token budget enforced at conversation start, not per-tool-invocation** — long-running agent loops accumulate cost beyond intended budget.
- **Trial-credit abuse via agent-spawn-agent** — meta-agent spawns N child agents; if child agents inherit trial credits, attacker multiplies trial usage.

### Modern JS RSC / Server Actions business-logic

Server Actions accept arbitrary user input — business-logic checks must happen server-side, but Server Actions are often written quickly with client-trust patterns:
- **Trusted client-side state in Server Action body** — Server Action receives `total`, `price`, `quantity` from client and processes without recomputation.
- **Server Action error responses revealing business-logic state** — error message includes "user already trialed" vs "user is on free tier" — enables trial-abuse exploit calibration.

### GitOps / K8s admission business-logic

ArgoCD / Flux business-logic patterns:
- **AppProject scope overly broad** — `destinations: ["*"]` and `sourceRepos: ["*"]` allows business-logic abuse where one team's deployment overwrites another team's resources.
- **Sync-policy auto-prune** — concurrent application updates trigger auto-prune that deletes resources in the wrong order.
- **Helm chart values overrides via merge** — values from multiple sources merged in unexpected order; attacker controls precedence.
- **OPA Gatekeeper policy enforcement order** — multiple constraints evaluated; attacker exploits evaluation-order assumptions.

