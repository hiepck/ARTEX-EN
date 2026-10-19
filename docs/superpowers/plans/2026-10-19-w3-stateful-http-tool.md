# W3: Stateful HTTP Tool for the Worker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the worker a session-aware HTTP tool (`http_request`) with full method/header/body control and a cookie jar that persists across calls within one intent run — something neither `Bash`+`curl` (stateless unless the model manually re-plumbs cookies) nor `WebFetch` (GET-only) currently offer well — as a strictly opt-in, default-unbound built-in tool that changes nothing for any agent that doesn't explicitly enable it.

**Architecture:** A new tool (`httpRequestTool`, a plain function returning `actool.CoreTool`, not a `*ToolSet` method — it has no dependency on the asset/exploration stores `ToolSet` otherwise carries) is built once per `Worker.execute` call, closing over a freshly-constructed `*http.Client` with a `net/http/cookiejar.Jar` and, when a recording proxy is configured, the same `Transport{Proxy, TLSClientConfig.RootCAs}` setup `server/customtool.go`'s existing `httpProxyTransport` uses for the "http" custom-tool kind — so this tool's traffic stays part of the evidence trail exactly like `WebFetch`/`Bash`'s. It is seeded into the `tools` catalog table like every other built-in tool, but forced to an empty agent binding by default (the existing `goal_met` precedent in `agent/toolcatalog.go`), so an operator must explicitly bind it to `worker` (or any agent) before it ever appears in a model's tool list.

**Tech Stack:** Go (`net/http`, `net/http/cookiejar`, `net/url`, `crypto/tls`, `crypto/x509`), the existing `agent`/`server` packages, the W0 integration harness (`server/harness_*_test.go`) for the end-to-end proof.

**Spec:** `docs/superpowers/specs/2026-10-17-core-engine-finding-capability-design.md` (workstream W3).

## Global Constraints

- **Opt-in by construction, not by a new setting**: unlike W1/W2, this workstream does NOT add a new `settingX`/`agent.XEnabled` toggle — the existing `tools` DB table (`Enabled`/`Agents` columns, `db/tools.go`) is already the per-tool, per-agent opt-in mechanism, and this plan uses it exactly as `goal_met` already does (`agent/toolcatalog.go:61`, `defaultUnbound`). Do not invent a parallel flag.
- **No new imports in `agent/worker.go`**: all HTTP-client-construction code (and its `net/http`, `net/http/cookiejar`, `net/url`, `crypto/tls`, `crypto/x509` imports) lives in the new `agent/tools_http.go` file. `worker.go` only calls two functions from that file (`buildHTTPToolClient`, `httpRequestTool`) and needs no new imports itself.
- **The tool constructor must be safe to call with a `nil` client**: `agent/toolcatalog.go`'s `builtinToolsByAgent()` builds a "read-only shell" `ToolSet` with nil stores specifically so tool constructors can be called purely to read `Name()/Description()/InputSchema()` for seeding, without ever invoking `Call()` (`agent/toolcatalog.go:26-35`'s own doc comment states this invariant). `httpRequestTool(nil)` must satisfy the same contract: constructible with a nil client, `Call()` only ever invoked on an instance built with a real client inside `Worker.execute`.
- **House style for the client/transport**: mirror `server/customtool.go:459-485`'s `httpProxyTransport` almost verbatim (`http.Transport{Proxy: http.ProxyURL(...)}`, CA pool loaded from a PEM file when present) — this repo has no existing `cookiejar` usage anywhere, so this tool is the first consumer; don't invent a different proxy-wiring shape.
- **Schema/tool-definition pattern**: use `actool.Build(actool.Spec{...})` directly (not a `*ToolSet` method, since this tool needs no asset/exploration store access) and the existing `obj()`/`str()` schema-builder helpers (`agent/tools.go:265-278`) for consistency with every other tool in this codebase.
- **`ReadOnly` is per-call, not per-tool**: `actool.Spec.ReadOnly` is `func(json.RawMessage) bool` (confirmed in the vendored `norma` module) — GET/HEAD/OPTIONS (or no method specified, which defaults to GET) are read-only; everything else is not. Do not hard-code a single bool for the whole tool.
- **`wireTools` touches global state**: calling it in a test (as Task 3's end-to-end test does, following the existing precedent in `server/tools_wire_test.go`) sets the package-level `agent.ToolResolve`, `agent.FindingTrafficBindingEnabled`, `agent.BurnSignalEnabled`, and `agent.ReachabilityEdgesEnabled` variables. Follow the existing test's own convention exactly: clean up only `agent.ToolResolve = nil` afterward — every other test in this codebase that cares about the other three variables' specific value already protects itself with its own save/restore (confirmed by reading `agent/burn_signal_test.go`, `agent/reachability_test.go`, `agent/finding_workflow_test.go`), so this plan does not need to restore them either.

## Review Focus

- **Cookies set by one response must actually be sent on the next request to the same host** — the single property this tool exists to provide. A test that only checks the tool compiles/responds, without proving a `Set-Cookie` from call 1 is echoed back as a `Cookie` header on call 2, would miss the point entirely.
- **`reset_cookies` must actually start a fresh session** (no stale cookie carried over), not merely be accepted and ignored.
- **The tool must be invisible to every agent until explicitly bound** — confirm via `BuiltinToolSeeds()`'s output, not just by reading the `defaultUnbound` map and assuming it works (map membership and the actual seed-emission code are two different things; prove the seed row itself carries an empty `Agents` list).
- **The tool's traffic must route through the same recording proxy as the rest of the worker's tools when one is configured** — a network-facing tool that silently bypasses the evidence-capture proxy would be a quiet regression in the chaining/evidence story this whole project has been building toward. (Proven by matching `server/customtool.go`'s exact transport-building logic; a live-proxy integration test is out of scope for this plan — the harness has no real recording proxy running — but the transport-construction code must be traceably identical to the already-shipped, already-trusted pattern.)
- **The end-to-end harness test (Task 3) must prove the OPT-IN mechanism for real** — bind the tool via `AddAgentToToolBinding` and show it then flows through the real `AugmentTools`/`ToolResolve` pipeline into a real worker run, not just that the Go function compiles.

---

### Task 1: `httpRequestTool` + `buildHTTPToolClient`

**Files:**
- Create: `agent/tools_http.go`
- Test: `agent/tools_http_test.go`

**Interfaces:**
- Consumes: `actool.Build(actool.Spec{...}) actool.CoreTool`, `permission.AskUser(msg string) permission.Decision` (vendored `norma` module, both already used elsewhere in this codebase); `obj(props map[string]any, required ...string) map[string]any` / `str(desc string) map[string]any` (`agent/tools.go:265,278`); `actool.Text(...)`, `actool.Errorf(...)`, `actool.Capture(tc, ...)` (the established result-building helpers, already used in `server/customtool.go:420-453`'s analogous "http" custom-tool kind).
- Produces: `func buildHTTPToolClient(proxyAddr, proxyCACert string) *http.Client` (a fresh cookie-jar-backed client; `proxyAddr==""` returns a plain direct client); `func httpRequestTool(client *http.Client) actool.CoreTool` (tool name `"http_request"`; `client==nil` is safe for metadata-only construction, `Call` on a nil client returns a clear error rather than panicking — defensive, even though in practice `Call` is never invoked on a nil-client instance per the Global Constraints). Used by Tasks 2 (seeding) and 3 (real wiring).

- [ ] **Step 1: Write the failing tests**

```go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func callHTTPTool(t *testing.T, tool actool.CoreTool, body string) map[string]any {
	t.Helper()
	res, err := tool.Call(context.Background(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Flatten()), &out); err != nil {
		t.Fatalf("decode: %v, raw=%s", err, res.Flatten())
	}
	return out
}

func TestHTTPRequestToolSessionPersistsCookiesAndResets(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 || hits == 3 {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
			io.WriteString(w, fmt.Sprintf("set-%d", hits))
			return
		}
		if c, err := r.Cookie("session"); err == nil && c.Value == "abc123" {
			io.WriteString(w, fmt.Sprintf("with-cookie-%d", hits))
			return
		}
		io.WriteString(w, fmt.Sprintf("no-cookie-%d", hits))
	}))
	defer srv.Close()

	client := buildHTTPToolClient("", "")
	tool := httpRequestTool(client)

	first := callHTTPTool(t, tool, fmt.Sprintf(`{"url":%q}`, srv.URL))
	if !strings.Contains(fmt.Sprint(first["body"]), "set-1") {
		t.Fatalf("call 1 body = %v", first["body"])
	}
	second := callHTTPTool(t, tool, fmt.Sprintf(`{"url":%q}`, srv.URL))
	if !strings.Contains(fmt.Sprint(second["body"]), "with-cookie-2") {
		t.Fatalf("call 2 did not carry the session cookie: %v", second["body"])
	}
	third := callHTTPTool(t, tool, fmt.Sprintf(`{"url":%q,"reset_cookies":true}`, srv.URL))
	if !strings.Contains(fmt.Sprint(third["body"]), "set-3") {
		t.Fatalf("call 3 (after reset_cookies) should look like a fresh session: %v", third["body"])
	}
	fourth := callHTTPTool(t, tool, fmt.Sprintf(`{"url":%q}`, srv.URL))
	if !strings.Contains(fmt.Sprint(fourth["body"]), "with-cookie-4") {
		t.Fatalf("call 4 should carry the cookie set by call 3: %v", fourth["body"])
	}
}

func TestHTTPRequestToolMethodHeaderBodyRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		out := fmt.Sprintf("%s|%s|%s", r.Method, r.Header.Get("X-Test"), string(body))
		io.WriteString(w, out)
	}))
	defer srv.Close()

	tool := httpRequestTool(buildHTTPToolClient("", ""))
	got := callHTTPTool(t, tool, fmt.Sprintf(`{"method":"POST","url":%q,"headers":{"X-Test":"hi"},"body":"payload"}`, srv.URL))
	if fmt.Sprint(got["body"]) != "POST|hi|payload" {
		t.Fatalf("round trip = %v", got["body"])
	}
	if got["status"] != float64(200) {
		t.Fatalf("status = %v", got["status"])
	}
}

func TestHTTPRequestToolRequiresURL(t *testing.T) {
	tool := httpRequestTool(buildHTTPToolClient("", ""))
	res, err := tool.Call(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Flatten(), "url") {
		t.Fatalf("expected a url-required error, got: %s", res.Flatten())
	}
}

func TestHTTPRequestToolReadOnlyByMethod(t *testing.T) {
	tool := httpRequestTool(nil)
	cases := map[string]bool{
		`{"url":"http://x"}`:                   true, // no method -> defaults to GET -> read-only
		`{"method":"GET","url":"http://x"}`:    true,
		`{"method":"HEAD","url":"http://x"}`:   true,
		`{"method":"POST","url":"http://x"}`:   false,
		`{"method":"DELETE","url":"http://x"}`: false,
	}
	for body, want := range cases {
		if got := tool.IsReadOnly(json.RawMessage(body)); got != want {
			t.Fatalf("%s: IsReadOnly = %v, want %v", body, got, want)
		}
	}
}
```

Add `actool "github.com/Autumn-27/norma/tool"` to the test file's imports (needed for the `callHTTPTool` helper's `tool actool.CoreTool` parameter type).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./agent/ -run TestHTTPRequestTool -v`
Expected: FAIL (`httpRequestTool`/`buildHTTPToolClient` undefined)

- [ ] **Step 3: Implement**

```go
package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"

	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
)

// buildHTTPToolClient constructs the stateful http.Client http_request uses for
// one worker run: a fresh cookie jar (persists cookies across calls within this
// run; Worker.execute builds a new one per claimed intent, so it resets on the
// next) and, when a recording proxy is configured, the same
// Transport{Proxy, TLSClientConfig.RootCAs} setup server/customtool.go's
// httpProxyTransport uses for the existing "http" custom-tool kind -- so this
// tool's traffic stays part of the evidence trail exactly like WebFetch/Bash's.
func buildHTTPToolClient(proxyAddr, proxyCACert string) *http.Client {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	if proxyAddr == "" {
		return client
	}
	pu, err := url.Parse("http://" + proxyAddr)
	if err != nil {
		return client
	}
	tr := &http.Transport{Proxy: http.ProxyURL(pu)}
	if proxyCACert != "" {
		if pem, err := os.ReadFile(proxyCACert); err == nil {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM(pem) {
				tr.TLSClientConfig = &tls.Config{RootCAs: pool}
			}
		}
	}
	client.Transport = tr
	return client
}

var httpReadOnlyMethods = map[string]bool{"GET": true, "HEAD": true, "OPTIONS": true}

// httpRequestTool is a session-aware HTTP client: cookies persist across calls
// within one worker run via client's cookiejar. client may be nil for
// metadata-only use (agent/toolcatalog.go's seeding path) -- Call is never
// invoked on that instance.
func httpRequestTool(client *http.Client) actool.CoreTool {
	return actool.Build(actool.Spec{
		Name:        "http_request",
		Description: "Send an HTTP(S) request with full method/header/body control, using a session that persists cookies across calls within this run (like a browser tab) -- use it for multi-step, session-dependent flows (login then authenticated request, CSRF token then submit) that WebFetch (GET-only) cannot drive. Routes through the same recording proxy as other tools, so responses remain part of the evidence trail.",
		Schema: obj(map[string]any{
			"method":        str("HTTP method, default GET."),
			"url":           str("the absolute http(s) URL to request."),
			"headers":       map[string]any{"type": "object", "description": "optional request headers, name -> value."},
			"body":          str("optional request body."),
			"reset_cookies": map[string]any{"type": "boolean", "description": "optional: clear this session's cookie jar before sending (start a fresh session)."},
		}, "url"),
		ReadOnly: func(in json.RawMessage) bool {
			var a struct {
				Method string `json:"method"`
			}
			_ = json.Unmarshal(in, &a)
			return httpReadOnlyMethods[strings.ToUpper(strings.TrimSpace(a.Method))] || strings.TrimSpace(a.Method) == ""
		},
		Permissions: func(_ context.Context, input json.RawMessage, _ permission.Context) permission.Decision {
			var a struct {
				URL string `json:"url"`
			}
			_ = json.Unmarshal(input, &a)
			return permission.AskUser("send HTTP request: " + a.URL)
		},
		Run: func(ctx context.Context, in json.RawMessage, tc *actool.ToolContext) (actool.Result, error) {
			if client == nil {
				return actool.Errorf("http_request is not available in this context"), nil
			}
			var a struct {
				Method       string            `json:"method"`
				URL          string            `json:"url"`
				Headers      map[string]string `json:"headers"`
				Body         string            `json:"body"`
				ResetCookies bool              `json:"reset_cookies"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if strings.TrimSpace(a.URL) == "" {
				return actool.Errorf("url is required"), nil
			}
			if a.ResetCookies {
				client.Jar, _ = cookiejar.New(nil)
			}
			method := strings.ToUpper(strings.TrimSpace(a.Method))
			if method == "" {
				method = "GET"
			}
			var bodyReader io.Reader
			if a.Body != "" {
				bodyReader = strings.NewReader(a.Body)
			}
			req, err := http.NewRequestWithContext(ctx, method, a.URL, bodyReader)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			for k, v := range a.Headers {
				req.Header.Set(k, v)
			}
			resp, err := client.Do(req)
			if err != nil {
				return actool.Errorf("the request failed: " + err.Error()), nil
			}
			defer resp.Body.Close()
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			out := map[string]any{"status": resp.StatusCode, "headers": resp.Header, "body": string(respBody)}
			b, _ := json.Marshal(out)
			return actool.Text(actool.Capture(tc, string(b))), nil
		},
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./agent/ -run TestHTTPRequestTool -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add agent/tools_http.go agent/tools_http_test.go
git commit -m "feat(agent): add a session-aware http_request tool"
```

---

### Task 2: Seed `http_request` as a default-unbound built-in tool

**Files:**
- Modify: `agent/toolcatalog.go`
- Test: `agent/toolcatalog_http_test.go`

**Interfaces:**
- Consumes: `httpRequestTool(client *http.Client) actool.CoreTool` (Task 1, called with `nil` here — pure metadata use, matching every other entry `builtinToolsByAgent()` constructs); `BuiltinToolSeeds() []ToolSeed` (`agent/toolcatalog.go:65`); `defaultUnbound map[string]bool` (`agent/toolcatalog.go:61`).
- Produces: `builtinToolsByAgent()["worker"]` includes `httpRequestTool(nil)`; `defaultUnbound["http_request"] = true`; `BuiltinToolSeeds()` therefore emits a `ToolSeed{Key: "http_request", Agents: []string{}}` entry (forced empty regardless of which agent's builtin set it came from — same mechanism `goal_met` already uses). Used by Task 3 (the seed is what `wireTools` inserts into the live `tools` table).

- [ ] **Step 1: Write the failing test**

```go
package agent

import "testing"

func TestHTTPRequestToolSeededDefaultUnbound(t *testing.T) {
	seeds := BuiltinToolSeeds()
	var found *ToolSeed
	for i := range seeds {
		if seeds[i].Key == "http_request" {
			found = &seeds[i]
			break
		}
	}
	if found == nil {
		t.Fatal("http_request not present in BuiltinToolSeeds()")
	}
	if len(found.Agents) != 0 {
		t.Fatalf("http_request.Agents = %v, want empty (default-unbound)", found.Agents)
	}
	if found.Desc == "" {
		t.Fatal("http_request seed has no description")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./agent/ -run TestHTTPRequestToolSeededDefaultUnbound -v`
Expected: FAIL (`http_request` not present in the seed list)

- [ ] **Step 3: Implement**

In `agent/toolcatalog.go`'s `builtinToolsByAgent()`, change the `"worker"` entry:
```go
"worker": append(ts.WorkerTools(), httpRequestTool(nil)),
```
And add to `defaultUnbound`:
```go
var defaultUnbound = map[string]bool{"goal_met": true, "http_request": true}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./agent/ -run TestHTTPRequestToolSeededDefaultUnbound -v`
Expected: PASS

- [ ] **Step 5: Run the whole `agent` package suite to confirm nothing else regressed**

Run: `go test ./agent/... -v`
Expected: PASS (or only the already-ledgered pre-existing `TestGraphOverviewExpandsAssociatedCompanyScope` failure — no new failures)

- [ ] **Step 6: Commit**

```bash
git add agent/toolcatalog.go agent/toolcatalog_http_test.go
git commit -m "feat(agent): seed http_request into the tool catalog, default-unbound"
```

---

### Task 3: Wire into `Worker.execute` + prove the opt-in end to end

**Files:**
- Modify: `agent/worker.go` (the `base := append(tsx.WorkerTools(), w.extraTools...)` region, `agent/worker.go:377`)
- Test: `server/http_tool_wiring_test.go` (new)

**Interfaces:**
- Consumes: `buildHTTPToolClient`/`httpRequestTool` (Task 1); `w.proxyAddr`/`w.proxyCACert` (`agent/worker.go:59-60`, already set via `Worker.SetProxy`); `wireTools(pg *db.DB, domainReg map[string]actool.CoreTool)` (`server/assembly.go:212`, already touched by every server-side plan in this project's history, unchanged here); `(d *DB) AddAgentToToolBinding(agentKey string, keys []string) error` (`db/tools.go`); the W0 harness's `setupHarness(t) (*Manager, *Task, *httptest.Server)`, `wireHarnessEngine(t, m, plannerProv, workerProv) *Engine`, `newScriptedProvider(...)`, `toolUseMsg`/`textMsg` helpers (`server/harness_fixture_test.go`, `server/harness_fakellm_test.go`, `server/harness_e2e_test.go`).
- Produces: nothing consumed by another task — this is the workstream's final deliverable, and the point where the W0 harness gets reused for the first time outside its own workstream, exactly as it was built for.

- [ ] **Step 1: Write the failing test**

```go
package server

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

func TestHTTPRequestToolOptInAndWorksEndToEnd(t *testing.T) {
	m, task, target := setupHarness(t)
	wireTools(m.pg, nil)
	t.Cleanup(func() { agent.ToolResolve = nil })
	if err := m.pg.AddAgentToToolBinding("worker", []string{"http_request"}); err != nil {
		t.Fatal(err)
	}

	plannerProv := newScriptedProvider(
		toolUseMsg("it_1", "add_intent", map[string]any{"summary": "harness probe: use http_request against the fake target"}),
		textMsg("intent dispatched"),
	)
	workerProv := newScriptedProvider(
		toolUseMsg("hr_1", "http_request", map[string]any{"url": target.URL}),
		toolUseMsg("rf_1", "record_fact", map[string]any{"summary": "harness-http-tool-marker: reached the fake target via http_request"}),
		textMsg("done"),
	)

	engine := wireHarnessEngine(t, m, plannerProv, workerProv)
	t.Cleanup(func() { engine.StopTask(task.ID) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go engine.Run(ctx, task)

	deadline := time.Now().Add(25 * time.Second)
	var lastState string
	for time.Now().Before(deadline) {
		facts, err := task.Store.ListByKind(db.KindFact, 10)
		n := 0
		if err == nil {
			n = len(facts)
			for _, f := range facts {
				if strings.Contains(string(f.Payload), "harness-http-tool-marker") {
					return
				}
			}
		}
		lastState = fmt.Sprintf("facts=%d err=%v", n, err)
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for scripted fact; last observed state: %s", lastState)
}
```

Add `"github.com/Autumn-27/artex/agent"` to the test file's imports (needed for `agent.ToolResolve = nil` in cleanup). Per the Global Constraints note, do not attempt to save/restore `agent.FindingTrafficBindingEnabled`/`agent.BurnSignalEnabled`/`agent.ReachabilityEdgesEnabled` here — matching `server/tools_wire_test.go`'s existing convention.

- [ ] **Step 2: Run test to verify it fails**

Run: `ARTEX_PG_DSN=<your throwaway DSN> go test ./server/ -run TestHTTPRequestToolOptInAndWorksEndToEnd -v`
Expected: FAIL — either a scripted-tool-call rejection (since `http_request` doesn't exist in the worker's tool list yet) or a timeout with no matching fact. Confirm it is NOT a compile error unrelated to the feature (e.g. a typo in the harness helper names) before moving on — if it is, fix the typo, not the feature.

- [ ] **Step 3: Implement the worker-side wiring**

In `agent/worker.go`, change:
```go
base := append(tsx.WorkerTools(), w.extraTools...)
```
to:
```go
base := append(tsx.WorkerTools(), w.extraTools...)
base = append(base, httpRequestTool(buildHTTPToolClient(w.proxyAddr, w.proxyCACert)))
```
(placed immediately after the existing line, before the `defaultToolsExcept(...)` append that follows it) — a fresh client (fresh cookie jar) is built every time `execute` runs, i.e. once per claimed intent, matching the plan's "resets on the next intent" design.

- [ ] **Step 4: Run test to verify it passes**

Run: `ARTEX_PG_DSN=<your throwaway DSN> go test ./server/ -run TestHTTPRequestToolOptInAndWorksEndToEnd -v`
Expected: PASS

- [ ] **Step 5: Run the whole `server` and `agent` package suites to confirm nothing else regressed**

Run: `ARTEX_PG_DSN=<your throwaway DSN> go test ./agent/... ./server/... -p 1 -v`
Expected: PASS (or only the already-ledgered pre-existing failures from prior workstreams — `TestGraphOverviewExpandsAssociatedCompanyScope`, `TestDeepenFindingCreatesAuditedIntentAndRevivesTask`, `TestTaskMetadataPatchReturnsRenameAndPin` — no new failures)

- [ ] **Step 6: Commit**

```bash
git add agent/worker.go server/http_tool_wiring_test.go
git commit -m "feat(agent): append http_request to the worker's tool list"
```
