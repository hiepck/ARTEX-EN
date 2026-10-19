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
	"time"

	actool "github.com/Autumn-27/norma/tool"
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

// TestHTTPRequestToolSessionPersistsCookiesAndResets proves cookie persistence
// AND that reset_cookies actually clears the jar, by using a distinct,
// traceable cookie value per session rather than one constant value: a server
// that always sets the same value (or always answers "has a cookie") cannot
// tell a cleared jar from a full one, since the value looks identical either
// way. Here, each time the server sees no incoming cookie it mints a NEW
// unique value and reports "nocookie"; when it does see one, it echoes the
// exact value back. A broken (no-op) reset would make call 3 echo session 1's
// value instead of reporting "nocookie", and call 4 would then echo session
// 1's value instead of session 2's — both are checked explicitly below.
func TestHTTPRequestToolSessionPersistsCookiesAndResets(t *testing.T) {
	var sessions int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("session"); err == nil {
			io.WriteString(w, "cookie="+c.Value)
			return
		}
		sessions++
		value := fmt.Sprintf("session-%d", sessions)
		http.SetCookie(w, &http.Cookie{Name: "session", Value: value})
		io.WriteString(w, "nocookie")
	}))
	defer srv.Close()

	client := buildHTTPToolClient("", "")
	tool := httpRequestTool(client)

	first := callHTTPTool(t, tool, fmt.Sprintf(`{"url":%q}`, srv.URL))
	if fmt.Sprint(first["body"]) != "nocookie" {
		t.Fatalf("call 1 (fresh client, no cookie yet) body = %v, want nocookie", first["body"])
	}
	second := callHTTPTool(t, tool, fmt.Sprintf(`{"url":%q}`, srv.URL))
	if fmt.Sprint(second["body"]) != "cookie=session-1" {
		t.Fatalf("call 2 did not carry session-1's cookie: %v", second["body"])
	}
	third := callHTTPTool(t, tool, fmt.Sprintf(`{"url":%q,"reset_cookies":true}`, srv.URL))
	if fmt.Sprint(third["body"]) != "nocookie" {
		t.Fatalf("call 3 (after reset_cookies) should carry no cookie, got: %v", third["body"])
	}
	fourth := callHTTPTool(t, tool, fmt.Sprintf(`{"url":%q}`, srv.URL))
	if fmt.Sprint(fourth["body"]) != "cookie=session-2" {
		t.Fatalf("call 4 should carry session-2's (post-reset) cookie, not session-1's: %v", fourth["body"])
	}
}

// TestBuildHTTPToolClientHasBoundedTimeout mirrors server/customtool.go's
// runHTTPTool, which bounds every request with both a context timeout AND
// client.Timeout: 30s. Without a client-level timeout, a hung target would
// block indefinitely, and because http_request is not concurrency-safe
// (IsConcurrencySafe defaults false), that stalls the worker's whole tool
// queue, not just this one call.
func TestBuildHTTPToolClientHasBoundedTimeout(t *testing.T) {
	client := buildHTTPToolClient("", "")
	if client.Timeout <= 0 {
		t.Fatalf("buildHTTPToolClient's client.Timeout = %v, want a bounded positive timeout", client.Timeout)
	}
	if client.Timeout != 30*time.Second {
		t.Fatalf("client.Timeout = %v, want 30s (matching server/customtool.go's runHTTPTool default)", client.Timeout)
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
