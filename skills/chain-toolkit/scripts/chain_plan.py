#!/usr/bin/env python3
"""chain_plan.py — rank next-hop chain candidates per finding (stateless, JSON in/out).

Reads a findings list from stdin and writes ranked chain candidates to stdout.

Input  (stdin):  {"findings": [{"id": <int>, "vulnclass": "<str>", "severity": "<str>"}, ...]}
Output (stdout): {"candidates": [{"finding_id": <int>, "vulnclass": "<str>",
                                  "next": [{"hop": "<desc>", "toward": "<label>"}, ...]}, ...]}

An unknown vulnclass yields that finding with an empty "next" list (never an error).

The ANCHORS_BY_CLASS table is copied verbatim from penred tools/chain_plan.py; the
second tuple element (penred's suggested sub-agent) becomes the human-readable
"toward" hint, since ARTEX has no sub-agents — the worker pursues the hop directly.
"""
from __future__ import annotations

import json
import sys

# Per-class chain anchors (verbatim from penred rules/chain-table.md "Per-Class
# Chain Anchors"). Each entry: class -> [(anchor_description, hop_label)].
ANCHORS_BY_CLASS: dict[str, list[tuple[str, str]]] = {
    "open-redirect": [
        ("OAuth `redirect_uri` reflection — try `redirect_uri=<your-domain>`; if auth code delivered, ATO chain confirmed", "oauth-hunter"),
        ("`returnTo` / `next` / `continue` post-auth (CVE-2025-67716 Auth0 nextjs-auth0 pattern) — try `returnTo=javascript:alert(document.cookie)`", "oauth-hunter"),
        ("SAML RelayState / OIDC `post_logout_redirect_uri` — try `RelayState=<svg onload=...>`", "oauth-hunter"),
        ("Login flow CSRF anchor — does redirect happen post-login? Self-XSS on redirect target + login CSRF = ATO", "csrf-hunter + xss-hunter"),
        ("Cookie tossing prerequisite — does redirect target a sibling subdomain you control?", "subdomain-takeover + auth-tester"),
    ],
    "cors": [
        ("Credentialed authenticated endpoint — find `Access-Control-Allow-Credentials: true`; without this, CORS misconfig is informational", "cors-hunter"),
        ("Sensitive data endpoint — does over-permissive origin reach `/api/me`, `/api/users/<id>`, billing, secrets? Document exfil", "idor-hunter"),
        ("Origin reflection + `null` origin — sandboxed iframe / data: URI bypasses", "cors-hunter"),
        ("Subdomain wildcard regex flaw — `https://target.com.attacker.com`, `https://nottarget.com`", "cors-hunter"),
        ("Cross-origin postMessage handler — find unvalidated `addEventListener('message',...)` and abuse as data-theft sink", "xss-hunter"),
    ],
    "info-disclosure": [
        ("Bundle/source/config containing OAuth secrets → client_secret + missing PKCE = code interception chain", "oauth-hunter"),
        ("Stack trace revealing internal IPs / service names → SSRF target list", "ssrf-hunter"),
        ("Debug endpoint returning request headers / session tokens → IDOR / session fixation", "idor-hunter + auth-tester"),
        ("`.git`, `.env`, `backup.tar.gz`, `wp-config.php`, `phpinfo`, `/server-status` → DB creds / signing keys / cloud creds", "privilege-escalation + oauth-hunter"),
        ("Cloud metadata reachable via XSS context (window.fetch to 169.254.169.254) → IMDS theft", "xss-hunter"),
        ("API key in JS bundle with active scope — verify scope; if cross-user data → IDOR-via-key", "idor-hunter"),
    ],
    "csrf": [
        ("CSRF on password / email / phone change → ATO chain (changes the recovery vector)", "auth-tester"),
        ("CSRF on MFA disable / second-factor enrollment → MFA bypass", "business-logic + auth-tester"),
        ("CSRF on role change / permission grant / team invite → privilege escalation", "privilege-escalation"),
        ("CSRF on payment-method change / withdrawal address → financial impact", "business-logic"),
        ("CSRF on OAuth client registration / API key creation → backdoor-credential chain", "oauth-hunter"),
    ],
    "subdomain-takeover": [
        ("Subdomain is OAuth `redirect_uri` / SAML ACS / OIDC issuer — claim → ATO chain", "oauth-hunter"),
        ("Parent domain shares cookies (`.target.com`) → cookie tossing → session fixation → ATO", "auth-tester"),
        ("Subdomain has wildcard cert → MitM / TLS-confusion chain", "config-auditor"),
        ("Subdomain referenced from prod domain JS (CDN, asset, config) → trusted-domain phishing", "info-disclosure"),
        ("Subdomain in CSP `script-src` → CSP bypass on parent → stored XSS escalation", "xss-hunter"),
    ],
    "xxe": [
        ("SSRF via XXE → cloud metadata (SYSTEM 'http://169.254.169.254/...') → IAM creds", "ssrf-hunter"),
        ("OOB exfil to attacker DTD → blind XXE on cookie/config/private files", "xxe-hunter"),
        ("SAML XXE (assertion parsing) → authentication bypass via signed-assertion forgery", "oauth-hunter"),
        ("DOCX / XLSX / SVG XXE upload — payload survives upload pipeline → triggers in admin viewer", "file-upload + xss-hunter"),
        ("PHP wrapper / Java JNDI — `php://filter`, `jar://`, `ldap://` for source code / classloader RCE", "rce-hunter"),
    ],
    "file-upload": [
        ("Upload to web root + executable → web shell → RCE", "rce-hunter"),
        ("Upload SVG / HTML rendered inline → stored XSS in viewer context", "xss-hunter"),
        ("Path traversal in filename → overwrite config / cron / .ssh/authorized_keys → privesc / RCE", "rce-hunter + privilege-escalation"),
        ("Upload metadata renders in admin panel (filename, EXIF) → stored XSS in admin context → ATO", "xss-hunter"),
        ("Upload triggers server-side processor (PDF, image resize, AV) → SSRF / RCE via processor CVE", "rce-hunter + ssrf-hunter"),
    ],
    "race-condition": [
        ("Race on financial action (transfer, withdrawal, balance debit) → quantify $", "business-logic"),
        ("Race on coupon / gift-card / referral redemption → quantify (free-product × N)", "business-logic"),
        ("Race on one-shot tokens (password-reset, invite-accept, MFA-enrollment) → ATO if redeemed twice", "auth-tester"),
        ("Race on file write check → TOCTOU → privilege escalation / RCE", "rce-hunter + privilege-escalation"),
        ("Race on rate-limit / quota check → bypass quota → mass enumeration → IDOR amplification", "idor-hunter"),
    ],
    "business-logic": [
        ("Public archive / share-with-admin trigger (listmonk pattern) — manipulated artifact → admin-context render", "xss-hunter"),
        ("State carries to other context — manipulated price stored server-side → admin panel source-of-truth", "xss-hunter + privilege-escalation"),
        ("Workflow skip → access feature you didn't pay for — quantify ($premium × users)", "business-logic"),
        ("Negative / huge values → integer overflow / sign flip → financial chain", "business-logic"),
        ("Time-of-check / time-of-use on balance → race-condition chain", "race-condition"),
    ],
    "privilege-escalation": [
        ("Mass-assignment to set role/permission/tenant — does escalated account see other-tenant data? → IDOR amplification", "idor-hunter"),
        ("JWT claim manipulation works — verify which other claims are unprotected (sub, aud, iss) → cross-tenant chain", "oauth-hunter"),
        ("Admin endpoint reachable but rate-limited — confirm full admin actions, not just GET", "privilege-escalation"),
        ("Forced-browsing admin URL works → check write operations (DELETE, PATCH) too", "privilege-escalation"),
        ("HTTP method override → bypass auth — try same trick on every other admin endpoint", "auth-tester"),
    ],
}


def plan(findings: list[dict]) -> list[dict]:
    out = []
    for f in findings:
        vc = str(f.get("vulnclass", "")).lower()
        anchors = ANCHORS_BY_CLASS.get(vc, [])
        nxt = [{"hop": desc, "toward": label} for (desc, label) in anchors]
        out.append({"finding_id": f.get("id"), "vulnclass": vc, "next": nxt})
    return out


def main() -> int:
    try:
        data = json.load(sys.stdin)
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"invalid json: {e}"}), file=sys.stderr)
        return 2
    print(json.dumps({"candidates": plan(data.get("findings", []))}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
