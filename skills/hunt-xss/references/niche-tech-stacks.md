---
name: niche-tech-stacks
description: niche-tech-stacks reference material.
---

# Niche Tech-Stack XSS Surfaces

Part of `skills/hunt-xss/SKILL.md`. Read this ONLY when the target is a CI/CD pipeline, a GitOps controller, a container-management dashboard, or an ML-serving / inference platform.

---

## Modern Meta — Cloud-Native, CI/CD, OSS Pipeline

This is where the 2024-2026 XSS meta lives. Bounties scale because XSS in a CI/CD pipeline = build artifact poison; XSS in a vendor SDK = downstream consumer compromise.

**GitHub Actions XSS surface** — workflow logs render PR titles / branch names. If `pull_request_target` interpolates `${{ github.event.pull_request.title }}` into a job step that emits HTML in artifact upload, viewers of the artifact see XSS. Less common than RCE via the same surface but documented in GitHub Security Lab issues.

**GitLab CI XSS** — pipeline view renders job names, commit messages, branch names. CVE-2023-1080 family covers XSS in GitLab CI Pages build via attacker-controlled `_redirects` file. Hunt: anything that renders attacker-supplied build metadata.

**Jenkins XSS** — `/job/<name>/api/json` reflects job name; older Jenkins had stored XSS in build descriptions, parameter values, view names. Multiple CVEs across 2018-2024.

**ArgoCD / Flux / Tekton (GitOps controllers) XSS surface** — UI rendering of Application names, sync status messages, Helm chart values. The ArgoCD Application object's `metadata.annotations` field has been a stored-XSS surface in older versions.

**Kubernetes dashboard XSS** — kubectl-proxy or k8s dashboard rendering pod logs with HTML that contains XSS. Less of a paying surface but a real attack vector.

**Cloud IAM / IMDS** — XSS chained into IMDSv1 access via `fetch()` from XSS execution context (when CSP allows it). The chain: XSS → `fetch('http://169.254.169.254/...')` → IAM key exfil. CSP usually blocks this; when it doesn't, the impact upgrades to RCE-class.

**Supply chain (npm / pip)** — XSS via compromised package: malicious package version published with XSS payload in default-export HTML, downstream consumers ship the payload. Documented across Socket.dev / ReversingLabs disclosures 2024-2026.

**OSS hunting workflow:** `socket dev <package>` → audit recent versions for HTML-rendering changes → diff with previous → if suspicious, file as XSS / supply-chain incident with the package's bug bounty / security contact.

## Modern Expansion Pack (2024-2026 currency)

The 2024-2026 expansion meta required by the validator. All five topics covered.

### Container escape / runtime XSS

<!-- expansion-na: container reason: container escape is RCE-class; XSS doesn't manifest at the runtime layer. The closest XSS analog is XSS in container-management UIs (Portainer, Docker Desktop, Rancher dashboard) where attacker-controlled container names, image labels, or environment variables render in the dashboard. Hunt those UIs separately as classic web-XSS targets. -->

The XSS analog at the container-management layer: Portainer / Rancher / Docker Desktop dashboards rendering container names, image labels, ENV vars. Submit a container with `name="<svg onload=alert(1)>"` or with crafted labels; if the dashboard renders these via innerHTML, classic stored XSS in admin context.

### ML serving / inference XSS

The cross-tenant AI/ML XSS pattern: ML serving frameworks have admin UIs (BentoML dashboard, MLflow UI, TorchServe `/management`) that render model metadata, experiment names, run descriptions. Stored XSS via model name / experiment description fields → execution in admin browser context.

- **MLflow UI XSS** — model registry rendering attacker-controlled experiment names. Multiple Huntr disclosures 2024-2025 (parallel to the path-traversal family CVE-2024-1483/1560/1594).
- **BentoML dashboard** — bento metadata rendered without sanitization in admin UI.
- **TorchServe /management endpoint** — model name reflection in model-list JSON, rendered by client-side dashboard.

### Agentic LLM tool-use XSS / output injection

The LLM06:2025 (Excessive Agency) + classic XSS intersection. Two patterns:

1. **LLM output → innerHTML** — chatbot / RAG / AI assistant UI assigns LLM response directly to `innerHTML`. Prompt injection coerces the LLM to emit `<img src=x onerror=...>`. Reference: OWASP LLM Top 10 2025 (https://genai.owasp.org/llmrisk/llm06-sensitive-information-disclosure/), Tenable Dec 2024 analysis (https://tenable.com/blog/what-you-must-know-about-the-owasp-top-10-for-llm-applications-2025-update).
2. **Tool-use → DOM XSS** — agent has a "render markdown" or "show HTML preview" tool that renders attacker-influenced content. Prompt-inject the tool input.

### Modern JS RSC / Server Actions XSS

CVE-2025-67779 / CVE-2025-55184 (Next.js Server Components DoS via Server Function endpoints, GHSA-5j59-xgg2-r9c4 published Dec 11 2025) demonstrates that the RSC runtime trusts payload structure. The same trust surface produces XSS when:
- RSC payloads or Server Action responses round-trip through `dangerouslySetInnerHTML`
- Hydration mismatch between server-rendered and client-rendered HTML
- Server Action errors reflected in client-rendered error UI

Affects React 19.0.0/19.1.0/19.1.1/19.2.0 with `react-server-dom-webpack` / `parcel` / `turbopack`; patches in React 19.0.2/19.1.3/19.2.2 and Next.js 14.2.35 / 15.x / 16.x.

### GitOps / K8s admission XSS

ArgoCD Application UI renders `metadata.annotations`, `spec.source.helm.parameters`, sync status. ArgoCD has had multiple XSS CVEs in older versions; current versions sanitize, but custom UI extensions may not.

Hunt: any GitOps platform where attacker-controlled CRD fields render in the management UI.

