---
name: source-code-review
description: source-code-review reference material.
---

# RCE Reference: Source Code Review Patterns

Part of `skills/hunt-rce/SKILL.md`. Read this ONLY when you have source/repo access to the target (OSS bug, internal pentest, in-scope GitHub org) -- never useful for pure black-box hunting.

---

## Source Code Review Patterns

When you have repo access (OSS bug, internal pentest, in-scope GitHub org), grep is faster than dynamic testing.

### Semgrep rules (paste into `.semgrep.yml`)

```yaml
rules:
  - id: rce-shell-true-with-input
    pattern-either:
      - pattern: subprocess.$F(..., shell=True, ...)
      - pattern: subprocess.run(..., shell=True, ...)
      - pattern: os.system($X)
      - pattern: os.popen($X)
    message: |
      shell=True or os.system with any string concatenation is RCE-equivalent
      if any portion of the command is user-influenced. Use shlex.quote or
      subprocess with list-form arguments, never shell=True.
    severity: ERROR
    languages: [python]
```

```yaml
rules:
  - id: rce-java-readobject-no-filter
    pattern: |
      $S = new ObjectInputStream(...);
      ...
      $S.readObject();
    pattern-not: |
      $S = new ObjectInputStream(...);
      ...
      $S.setObjectInputFilter(...);
      ...
      $S.readObject();
    message: |
      ObjectInputStream.readObject without setObjectInputFilter is direct path
      to ysoserial gadget chain RCE. Add an allowlist filter or migrate to
      JSON/Protobuf.
    severity: ERROR
    languages: [java]
```

```yaml
rules:
  - id: rce-cli-arg-injection-no-sentinel
    pattern-either:
      - pattern: |
          exec.Command("git", $REV, ...)
      - pattern: |
          subprocess.run(["git", $REV, ...])
      - pattern: |
          subprocess.run(["curl", $URL, ...])
    message: |
      First positional argument to git/curl without `--` sentinel allows
      argument injection (--upload-pack=, --engine=, --config=, etc.).
      See CVE-2025-21613 (go-git), CVE-2026-40938 (Tekton, NVD-verified),
      CVE-2026-24685 (OpenProject, NVD-verified). Validate $REV/$URL
      doesn't start with `-`, or insert `"--"` before the first positional.
    severity: ERROR
    languages: [go, python]
```

```yaml
rules:
  - id: rce-template-render-string-with-input
    pattern-either:
      - pattern: render_template_string($X, ...)
      - pattern: jinja2.Template($X).render(...)
      - pattern: $T.from_string($X).render(...)
    message: |
      render_template_string with user-controlled template string is SSTI →
      RCE. Use render_template with a fixed template file and pass user data
      through the context kwargs, never the template body.
    severity: ERROR
    languages: [python]
```

```yaml
rules:
  - id: rce-actions-script-injection
    pattern-either:
      - pattern-regex: '\$\{\{\s*github\.event\.pull_request\.(title|body|head\.ref|head\.label)\s*\}\}'
      - pattern-regex: '\$\{\{\s*github\.head_ref\s*\}\}'
    message: |
      Direct interpolation of pull_request.title/body/head_ref into a `run:`
      block is GitHub Actions script injection (Cilium GHSL-2024-274, Ceph
      GHSA-p433-fp4g-pc2c, ansible.platform GHSA-fwqj-x86q-prmq). Use
      env: + reference $ENV_VAR in the script instead.
    severity: ERROR
    languages: [yaml]
    paths:
      include: ['.github/workflows/']
```

```yaml
rules:
  - id: rce-pickle-from-request
    pattern-either:
      - pattern: pickle.loads($X)
      - pattern: pickle.load($X)
      - pattern: cPickle.loads($X)
    message: |
      pickle.loads on attacker-controlled bytes is universal RCE. See
      CVE-2025-27520 / CVE-2025-32375 (BentoML), CVE-2024-2912 (BentoML),
      and the entire ML-serving family. Replace with safetensors, JSON, or
      explicit allowlist via Unpickler.find_class override.
    severity: ERROR
    languages: [python]
```

```yaml
rules:
  - id: rce-langchain-python-repl
    pattern-either:
      - pattern: PythonREPLTool(...)
      - pattern: PythonAstREPLTool(...)
      - pattern: create_pandas_dataframe_agent(...)
      - pattern: VectorSQLDatabaseChain(...)
    message: |
      PythonREPLTool / PandasDataFrameAgent execute LLM-generated Python in
      the host process with full filesystem/network/env access. CVE-2025-68613
      (NVD-verified CVSS 9.8). Use sandbox runtimes (E2B, gVisor, Docker)
      or AST-filter dangerous imports. Never expose to user-influenced input
      including RAG context.
    severity: ERROR
    languages: [python]
```

### ast-grep patterns

```bash
# Java: readObject without setObjectInputFilter
ast-grep --pattern '$S.readObject()' --lang java -A 0

# Python: subprocess with shell=True and any non-literal arg
ast-grep --pattern 'subprocess.$F($CMD, shell=True)' --lang python

# Node.js: child_process.exec with template literal (string concatenation)
ast-grep --pattern 'child_process.exec(`$$$`)' --lang js

# Go: exec.Command with user-controlled rev as first positional (no `--` sentinel)
ast-grep --pattern 'exec.Command("git", $REV, $$$)' --lang go

# JS: Lodash _.merge with user input (prototype pollution sink)
ast-grep --pattern '_.merge($DST, $SRC)' --lang js

# Python: yaml.load (not safe_load)
ast-grep --pattern 'yaml.load($X)' --lang python

# Ruby: YAML.load (not safe_load)
ast-grep --pattern 'YAML.load($X)' --lang ruby

# Python: pickle.loads on request data (BentoML pattern)
ast-grep --pattern 'pickle.loads($BODY)' --lang python
```

### ripgrep one-liners

```bash
# Find every readObject without nearby filter (Java)
rg -n -B 5 -A 5 'readObject\(\)' --type java | rg -v 'setObjectInputFilter|safeReadObject'

# Find shell=True interpolations (Python)
rg -n 'shell=True' --type py -B 2 -A 2 | rg -B 4 -A 4 '"\s*\+|\.format\(|f"|%\s*\(|{[^}]*}'

# Find SSTI sinks taking variables, not literals (Python Jinja)
rg -n 'render_template_string\([^"]' --type py

# Find raw SQL/CMD interpolation in Node template literals
rg -n 'exec\(`[^`]*\$\{' --type js

# Find unsafe pickle / marshal / cPickle
rg -n -e 'pickle\.loads?\(' -e 'marshal\.loads\(' -e 'cPickle\.loads?\(' --type py

# Find SnakeYaml insecure constructor (Java)
rg -n 'new Yaml\(\)|new SnakeYaml\(\)' --type java

# Find GitHub Actions script injection sources
rg -n 'github\.event\.pull_request\.(title|body|head\.ref|head\.label)|github\.head_ref' .github/workflows

# Find pull_request_target + checkout of head.sha (PPE)
rg -n -B 5 -A 20 'pull_request_target' .github/workflows | rg -B 3 -A 3 'head\.sha|head\.ref'

# Find argument injection candidates (no `--` sentinel)
rg -n 'exec\.Command\("(git|curl|tar|ssh)"' --type go | rg -v '"--"'
rg -n '\["(git|curl|tar)", [^"]' --type py

# Find LangChain REPL tools exposed to user input (CVE-2025-68613)
rg -n 'PythonREPLTool|PandasDataFrameAgent|create_pandas_dataframe_agent' --type py

# Find pickle-content-type acceptance (BentoML pattern)
rg -n 'application/vnd\..*\+pickle|application/x-python-pickle' --type py
```

### CodeQL hint

Use the standard `java/unsafe-deserialization` query (`UnsafeDeserializationQuery.qll`) for any Java target. Sources: any `RemoteFlowSource`. Sinks: `UnsafeDeserializationSink` (Kryo, XmlDecoder, XStream, SnakeYaml, JYaml, JsonIO, YAMLBeans, HessianBurlap, Castor, Burlap, Jackson, Jabsorb, Jodd JSON, Flexjson, Gson, JMS, ObjectInputStream).

Custom CodeQL predicate sketch for argument-injection detection (Python):

```ql
import python
import semmle.python.dataflow.new.DataFlow
import semmle.python.security.dataflow.CommandInjectionQuery

class CliArgInjection extends TaintTracking::Configuration {
  CliArgInjection() { this = "CliArgInjection" }
  override predicate isSource(DataFlow::Node src) {
    src instanceof RemoteFlowSource
  }
  override predicate isSink(DataFlow::Node sink) {
    exists(SubprocessCall c |
      c.getArgList().getElement(1) = sink.asExpr() and
      c.getArgList().getElement(0).getStringValue() in ["git","curl","tar","ssh","rsync"]
    )
  }
}
```

For Node.js, GitHub's pre-built `js/prototype-pollution-utility` and the academic `GHunter` / `Silent Spring` papers describe taint-tracking gadget detection that finds the universal Node gadgets (NPM CLI, Parse Server, Rocket.Chat).

For Python ML serving, write a custom predicate where sources are HTTP request bodies and sinks are `pickle.loads` / `pickle.load` calls — this catches the entire BentoML / TorchServe / Triton family.
