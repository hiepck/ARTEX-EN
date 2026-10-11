---
name: modern-expansion-platforms
description: modern-expansion-platforms reference material.
---

# Modern platform-specific info-disclosure deep-dives

Part of `skills/hunt-info-disclosure/SKILL.md`. Read this ONLY when the target runs one of the specific platforms covered here: shared-PID containers, an ML model-serving platform, an agentic LLM/MCP surface, Next.js (RSC/Server Actions), or a GitOps/K8s admission pipeline.

---

## Modern Expansion Pack (2024-2026 currency)

The 2024-2026 expansion meta required by the validator. All five topics covered.

### Container escape / runtime info-disclosure

<!-- expansion-na: container reason: container escape is RCE-class; info-disclosure analog at the runtime layer is process-cmdline disclosure (e.g., /proc/<pid>/cmdline accessible across containers in shared-PID-namespace setups) and Dgraph GHSA-95mq-xwj4-r47p /debug/pprof/cmdline pattern — both covered in Sub-technique F above. -->

The closest analog: container metadata / runtime info disclosure. Process command lines visible across PID namespaces (`/proc/<pid>/cmdline`) when `pid: host` is set in pod spec, or when `runc exec` exposes other-container info. Container labels (Docker / OCI labels) embedded in registry metadata can leak internal hostnames, repository paths.

### ML serving / inference info-disclosure

The cross-tenant AI/ML pattern — model registries leak credentials embedded in model metadata, feature stores expose training-data, vector DBs leak embeddings across tenants.

- **MLflow `/api/2.0/mlflow/runs/get`** — returns full run metadata including secret env vars, parameter values containing credentials.
- **Hugging Face model cards** with embedded API examples containing real keys (GitGuardian observed pattern).
- **BentoML** dashboard exposes bento metadata including build-time env vars.
- **Glances `/api/4/serverslist` (GHSA-r297-p3v4-wp8m)** — covered above; ML monitoring pipelines using Glances inherit the leak.

### Agentic LLM tool-use / output info-disclosure

LLM07:2025 System Prompt Leakage covers system-prompt extraction. LLM02:2025 Sensitive Information Disclosure covers training-data and runtime-data leakage. Practical attack:

- **PraisonAI WebSocket Gateway (GHSA-cfh6-vr3j-qc3g)** — `/info` exposes agent topology + tool definitions including credentials in tool config.
- **LLM RAG context leak** — agent reads sensitive document, repeats portions in subsequent (different-user) responses if context isn't tenant-scoped.
- **MCP server tool definitions exposed without auth** — see hunt-llm-ai.md for full coverage.
- **Chatbot debug responses** — error responses include retrieved-document snippets, prompt template contents, model reasoning traces.

### Modern JS RSC / Server Actions info-disclosure

- **Next.js Server Action error responses** sometimes include partial stack traces or environment data. Test malformed RSC payloads for verbose error responses.
- **Source maps in production** (`/_next/static/.../*.map`) reveal full source code, internal variable names, comments.
- **`__NEXT_DATA__` JSON inline in HTML** may include build-time env-var contents if `serverRuntimeConfig` and `publicRuntimeConfig` are misconfigured.
- **Vercel deployment metadata** (`/_vercel/insights`, `/.well-known/vercel`) sometimes exposes deployment IDs, build URLs.

### GitOps / K8s admission info-disclosure

- **ArgoCD Application UI** renders `metadata.annotations` and `spec.source.helm.parameters` — credentials in Helm values get displayed.
- **Tekton TaskRun spec** logs include parameter values; credentials in params get logged.
- **Helm chart values.yaml** committed to repo with real production credentials.
- **kustomize ConfigMap generators** that include `.env` files via `envs:` field.
- **Argo Workflows `argo-server` UI** exposes workflow parameters, secret references, status history.

