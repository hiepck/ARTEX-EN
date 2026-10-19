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
