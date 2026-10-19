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
