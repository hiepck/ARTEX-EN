// Package agent wires real LLM-driven planner and work agents (on top of the
// agent-core SDK) to the dual SQLite graph. See docs/ARTEX-architecture-design.md
// §4.3 (planner) and §4.4 (work agent).
//
// Provider configuration is read from the environment so the system runs with
// any Anthropic- or OpenAI-format endpoint. If no key is configured, FromEnv
// returns ok=false and the exploration engine stays idle (an LLM is required).
package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Autumn-27/artex/llmrec"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/compaction"
	"github.com/Autumn-27/norma/llm"
	acperm "github.com/Autumn-27/norma/permission"
	"github.com/Autumn-27/norma/transcript"
)

// Config describes the LLM backend resolved from the environment.
type Config struct {
	Format  llm.Format
	BaseURL string
	APIKey  string
	Model   string
	// Proxy routes all LLM requests through the given proxy URL (http/https/socks5,
	// optionally with user:pass@ credentials). Empty means direct — it does NOT
	// fall back to the standard *_PROXY environment variables.
	Proxy string
	// RatePerSecond / RatePerMinute cap the shared request rate across ALL agents
	// using the provider (0 = that window unlimited).
	RatePerSecond float64
	RatePerMinute float64
	// ContextWindowK is the model's context window in K tokens (user-configured),
	// used to size compaction thresholds. 0 = default; see CompactionWindow.
	ContextWindowK int
	// ThinkingType independently controls the thinking "switch" field (thinking.type):
	//   "" = do not send (the default, compatible with models that lack the field); "disabled" = explicitly off;
	//   "enabled" = on. Fully decoupled from ReasoningEffort -- some APIs have no thinking field and
	//   activate thinking with the effort parameter alone, so the two can be set independently.
	ThinkingType string
	// ReasoningEffort independently controls the thinking "effort" field:
	//   "" = do not send (the default); "low"/"medium"/"high"/"xhigh"/"max" = the corresponding effort.
	//   OpenAI maps it to the top-level reasoning_effort; Anthropic maps it to output_config.effort.
	ReasoningEffort string
	// Stream controls whether this profile uses the streaming (SSE) API. true (the default) = streaming;
	// false = genuinely non-streaming (sends stream:false, takes the complete JSON in one go, via
	// Provider.Complete). Non-streaming can sidestep a gateway's poor SSE implementation (empty frames,
	// dropped thinking frames) at the cost of losing live progress and live token counts during a run.
	// It maps to agentcore.Options.NonStreaming = !Stream.
	Stream bool
	// MaxTokens is the output cap (tokens) for one reply. 0 = do not send the field and let the server
	// default decide (the historical behaviour). It differs from ContextWindowK: the latter is the
	// model's total capacity, used locally only to compute the compaction threshold and never appearing
	// in a request, while this value is sent with every request. It maps to agentcore.Options.MaxTokens.
	MaxTokens int
	// MaxTokensField picks which request field name carries MaxTokens, only for format=openai:
	//   "" = max_tokens (the default); "max_completion_tokens" = the newer field.
	// OpenAI reasoning models (the o series / GPT-5) accept only the latter and reject max_tokens with
	// unsupported_parameter, while most compatible gateways accept only the former, so it is not inferred automatically and is left to the user to pick per endpoint.
	MaxTokensField string
	// SessionHeaderKey, when non-empty, makes every LLM request carry a custom HTTP header named by this
	// value whose value is [the current session's session id] (a chat session = conv-<id>, a worker =
	// exp<x>-worker-i<intent> and so on, see WorkerSessionID). It is for gateways doing prompt caching /
	// sticky routing by a session-id header.
	// Empty = do not send it. The value is attached to the request context by transcript.WithSessionID and
	// read by the RoundTripper, so even one shared provider can emit a different header value per session.
	SessionHeaderKey string
	// Retry holds this configuration's resolved retry parameters (profile override -> global policy ->
	// builtin defaults, resolved on the server side). The meaning of the three layers is in RetryConfig; a zero value = the builtin defaults throughout.
	Retry RetryConfig
}

// RetryConfig holds the retry parameters that travel with an LLM configuration. Every layer's "count"
// has uniform semantics: 0 = use the builtin default count; negative = disable that layer; >0 = use this
// value. Every layer's "interval": 0 = use that layer's original exponential backoff; >0 = use this fixed interval.
type RetryConfig struct {
	// ConnectAttempts/ConnectInterval: SDK connection retries (connection reset/timeout/429/5xx, before
	// the stream starts), mapping directly to llm.Config.MaxRetries / RetryInterval. Default 3 attempts, exponential from 0.5s (capped at 8s).
	ConnectAttempts int
	ConnectInterval time.Duration
	// EmptyAttempts/EmptyInterval: SDK empty-response retries (completed with no content block, openai
	// format only), mapping to llm.Config.EmptyResponseRetries / EmptyResponseInterval.
	// Default 2 attempts on the same exponential ladder.
	EmptyAttempts int
	EmptyInterval time.Duration
	// StreamAttempts/StreamInterval: same-provider safe-window retries -- a layer this project adds on top
	// of the SDK, replaying a dropped stream / overload / in-stream 429 only while "no output has been
	// delivered to the caller yet". The SDK cannot see it;
	// it is consumed by server/task_llm.go. Default 2 attempts, exponential from 0.5s (capped at 4s).
	StreamAttempts int
	StreamInterval time.Duration
}

// compaction window resolution bounds (in K tokens). Below the floor the
// threshold math (window − summary reserve − buffer) would go non-positive and
// compaction would fire every turn; above the cap it would never fire.
const (
	defaultWindowK = 200  // unset → assume a 200K window (Claude default)
	minWindowK     = 32   // floor so effectiveWindow stays comfortably positive
	maxWindowK     = 1000 // cap at 1M tokens (user request)
)

// CompactionWindow returns the model context window in TOKENS for compaction
// thresholds, resolved from the user-configured size (ContextWindowK). 0/unset →
// a 200K default; otherwise clamped to [32K, 1M] so compaction stays effective.
func (c Config) CompactionWindow() int {
	k := c.ContextWindowK
	if k <= 0 {
		k = defaultWindowK
	}
	if k < minWindowK {
		k = minWindowK
	}
	if k > maxWindowK {
		k = maxWindowK
	}
	return k * 1000
}

// compactionConfig builds the agent-core compaction config for a context window
// in tokens. agentcore.NewSession wires the summarizer (same provider) when this
// is set on Options.Compaction.
func compactionConfig(windowTokens int) *compaction.Config {
	if windowTokens <= 0 {
		windowTokens = defaultWindowK * 1000
	}
	return &compaction.Config{ContextWindow: windowTokens}
}

// FromEnv resolves the LLM provider config:
//
//	ARTEX_LLM_PROVIDER = anthropic|openai (default: inferred from keys)
//	ARTEX_LLM_MODEL    = model id        (default: per provider)
//	ARTEX_LLM_BASE_URL = endpoint        (optional)
//	ARTEX_LLM_PROXY    = proxy URL        (optional; http/https/socks5)
//	ANTHROPIC_API_KEY / OPENAI_API_KEY         = credentials
func FromEnv() (Config, bool) {
	prov := os.Getenv("ARTEX_LLM_PROVIDER")
	anthKey := os.Getenv("ANTHROPIC_API_KEY")
	oaiKey := os.Getenv("OPENAI_API_KEY")

	if prov == "" {
		switch {
		case anthKey != "":
			prov = "anthropic"
		case oaiKey != "":
			prov = "openai"
		default:
			return Config{}, false
		}
	}

	c := Config{
		BaseURL: os.Getenv("ARTEX_LLM_BASE_URL"),
		Model:   os.Getenv("ARTEX_LLM_MODEL"),
		Proxy:   strings.TrimSpace(os.Getenv("ARTEX_LLM_PROXY")),
		// Streaming by default; ARTEX_LLM_STREAM=false/0/off turns it off explicitly for non-streaming.
		Stream: !isFalsy(os.Getenv("ARTEX_LLM_STREAM")),
	}
	switch prov {
	case "openai":
		c.Format = llm.FormatOpenAI
		c.APIKey = oaiKey
		if c.Model == "" {
			c.Model = "gpt-4o"
		}
	case "openai-responses":
		c.Format = llm.FormatOpenAIResponses
		c.APIKey = oaiKey
		if c.Model == "" {
			c.Model = "gpt-5"
		}
	default:
		c.Format = llm.FormatAnthropic
		c.APIKey = anthKey
		if c.Model == "" {
			c.Model = "claude-opus-4-8"
		}
	}
	if c.APIKey == "" {
		return Config{}, false
	}
	return c, true
}

// ConfigFrom builds a Config from UI-provided strings (provider defaults to
// anthropic; model defaults per provider). Inputs are trimmed and the base URL
// is normalized to the API base the provider expects (the provider appends the
// endpoint path itself), so a full endpoint URL is tolerated.
func ConfigFrom(provider, model, baseURL, apiKey, proxy string) Config {
	c := Config{
		Model:   strings.TrimSpace(model),
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
		Proxy:   strings.TrimSpace(proxy),
		Stream:  true, // streaming by default; the caller overrides it per profile
	}
	switch strings.TrimSpace(provider) {
	case "openai":
		c.Format = llm.FormatOpenAI
		// provider appends "/chat/completions"; tolerate a full endpoint URL.
		c.BaseURL = strings.TrimRight(strings.TrimSuffix(c.BaseURL, "/chat/completions"), "/")
		if c.Model == "" {
			c.Model = "gpt-4o"
		}
	case "openai-responses":
		c.Format = llm.FormatOpenAIResponses
		// provider appends "/responses"; tolerate a full endpoint URL.
		c.BaseURL = strings.TrimRight(strings.TrimSuffix(c.BaseURL, "/responses"), "/")
		if c.Model == "" {
			c.Model = "gpt-5"
		}
	default:
		c.Format = llm.FormatAnthropic
		// provider appends "/v1/messages".
		c.BaseURL = strings.TrimRight(strings.TrimSuffix(c.BaseURL, "/v1/messages"), "/")
		if c.Model == "" {
			c.Model = "claude-opus-4-8"
		}
	}
	return c
}

// isFalsy reports whether an env-var string explicitly requests "off". Empty or
// unrecognized → false (so an unset var keeps the streaming default).
func isFalsy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "false", "off", "no":
		return true
	}
	return false
}

// Provider returns the short provider name ("anthropic"/"openai").
func (c Config) Provider() string {
	switch c.Format {
	case llm.FormatOpenAI:
		return "openai"
	case llm.FormatOpenAIResponses:
		return "openai-responses"
	}
	return "anthropic"
}

// NewProvider builds an llm.Provider from the config. When a rate is set, the
// limiter lives on the single provider instance — so planner + all workers +
// main agent (which share this provider) are bounded by one shared rate limit.
func (c Config) NewProvider() (llm.Provider, error) {
	client, err := quotaAwareHTTPClient(c.Proxy, c.SessionHeaderKey)
	if err != nil {
		return nil, err
	}
	lc := llm.Config{
		Format:     c.Format,
		BaseURL:    c.BaseURL,
		APIKey:     c.APIKey,
		Model:      c.Model,
		HTTPClient: client,
	}
	// The thinking switch and effort fields are passed through independently (empty = do not send that field). They are decoupled:
	// you may send only thinking.type, only effort, both, or neither.
	lc.ThinkingType = c.ThinkingType
	lc.ReasoningEffort = c.ReasoningEffort
	// The choice of field name for the output cap (empty = use max_tokens). The cap's "value" is not here:
	// it travels each round with agentcore.Options.MaxTokens, and the provider only decides which key it goes into.
	lc.MaxTokensField = c.MaxTokensField
	// The retry parameters have the same semantics as the SDK's (count 0=default/negative=off, interval 0=exponential backoff/>0=fixed) and are passed through as-is.
	lc.MaxRetries = c.Retry.ConnectAttempts
	lc.RetryInterval = c.Retry.ConnectInterval
	lc.EmptyResponseRetries = c.Retry.EmptyAttempts
	lc.EmptyResponseInterval = c.Retry.EmptyInterval
	if c.RatePerSecond > 0 || c.RatePerMinute > 0 {
		lc.RateLimit = &llm.RateLimit{PerSecond: c.RatePerSecond, PerMinute: c.RatePerMinute}
	}
	return llm.NewProvider(lc)
}

// IsQuotaExhaustedMessage deliberately recognizes only explicit balance,
// billing, credit, or quota-exhaustion signals. Generic 429/rate-limit text,
// authentication failures, network errors, and server failures are excluded.
var nonFailoverHTTPStatus = regexp.MustCompile(`(?:status(?:\s+code)?|http(?:\s+status)?)\s*[=:]?\s*(?:401|403|5\d\d)\b`)
var transientQuotaLimit = regexp.MustCompile(`(?i)(?:\b(?:rpm|tpm|rpd|qps)\b|quota[_\s-]*metric|rate[_\s-]*limit|too many requests|(?:requests?|tokens?)\s+(?:per|/)\s*(?:second|minute)|(?:per|/)\s*(?:second|minute)\s+(?:requests?|tokens?)|generate[_\s-]*requests[_\s-]*per[_\s-]*(?:minute|second)|tokens?[_\s-]*per[_\s-]*(?:minute|second))`)

func IsQuotaExhaustedMessage(message string) bool {
	message = strings.ToLower(message)
	// Authentication/authorization and provider-side 5xx failures never rotate,
	// even when a gateway happens to echo a quota-looking phrase in the body.
	if nonFailoverHTTPStatus.MatchString(message) {
		return false
	}
	// Provider APIs frequently describe an ordinary rate limit as "quota
	// exceeded", especially Google-style responses containing a quota metric.
	// These limits recover with time and must stay on the current provider.
	if transientQuotaLimit.MatchString(message) {
		return false
	}
	markers := []string{
		"insufficient_quota", "quota_exceeded", "quota exceeded", "quota exhausted",
		// "quota is exhausted" (with the copula) is what some gateways emit
		// ("Coding Plan quota is exhausted and wallet balance is insufficient");
		// the marker above only matches the copula-less form.
		"quota is exhausted",
		"exceeded your current quota", "billing_hard_limit_reached",
		"billing hard limit", "billing_not_active", "credit balance", "insufficient credit",
		"insufficient balance", "balance is too low", "payment required", "status 402",
		// Plan-level exhaustion, not a transient rate limit: OpenCode Zen's Go plan
		// answers 429 {"type":"GoUsageLimitError","message":"Go usage limit exceeded"}
		// with limitName weekly, and its coding plan answers 402 with
		// coding_plan_exhausted + "wallet balance is insufficient". Both mean "this
		// account cannot serve another call until the plan resets", so failover must
		// rotate off the profile — a 429 with a rate-limit phrase is filtered out
		// above, so these markers cannot swallow an ordinary throttle.
		"usage limit exceeded", "usage_limit_exceeded",
		"coding_plan_exhausted", "insufficient_balance",
		// Chinese markers are DATA, not UI text: these are the literal phrases Chinese LLM
		// gateways (DeepSeek, Zhipu, Moonshot and the like) put in their error bodies when an
		// account runs out of credit. Dropping them would stop failover from recognizing an
		// exhausted account on those providers.
		"余额不足", "额度不足", "额度已用尽", "欠费",
	}
	for _, marker := range markers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	// gRPC RESOURCE_EXHAUSTED is overloaded for both account quota and ordinary
	// request-rate limiting. Preserve it as an explicit exhaustion signal only
	// when the same error does not identify a transient rate limit.
	return strings.Contains(message, "resource_exhausted") &&
		!strings.Contains(message, "rate limit") &&
		!strings.Contains(message, "too many requests")
}

// quotaAwareTransport preserves Norma's normal retry behavior except for a 429
// whose body explicitly says the account quota/balance is exhausted. Norma's
// retry loop treats every 429 as transient; normalizing only that response to
// 402 lets a task router fail over immediately while retaining the original
// response body for provider-specific classification and audit logs.
type quotaAwareTransport struct {
	base http.RoundTripper
	// sessionHeaderKey, when non-empty, is the HTTP header name each request
	// carries; its value is the session id read from the request context. Empty
	// disables it. See Config.SessionHeaderKey.
	sessionHeaderKey string
}

func (t quotaAwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Custom session-id header: name is user-configured, value is THIS run's
	// session id (norma stashes it on the context via transcript.WithSessionID).
	// Stable across a session's turns and distinct across sessions — exactly what
	// a session-keyed prompt cache wants. Skipped when no session id is present.
	if t.sessionHeaderKey != "" {
		if sid := transcript.SessionIDFrom(req.Context()); sid != "" {
			req.Header.Set(t.sessionHeaderKey, sid)
		}
	}
	// When LLM recording is on, the Recorder puts a Capture on the context so the
	// raw wire bodies can be persisted. This is the only layer that still sees
	// them: norma builds the request body internally and decodes the SSE response
	// before either reaches the recorder.
	capt := llmrec.CaptureFrom(req.Context())
	capt.SetRequest(requestBodySnapshot(req))

	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	// Tee rather than read: a 200 is an SSE stream that must keep streaming. The
	// 429 branch below reads through this wrapper, so its body lands in the
	// capture before being replaced.
	resp.Body = capt.TeeResponse(resp.StatusCode, resp.Body)

	if resp.StatusCode != http.StatusTooManyRequests {
		return resp, nil
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	if readErr != nil {
		return resp, nil
	}
	if IsQuotaExhaustedMessage(string(body)) {
		resp.StatusCode = http.StatusPaymentRequired
		resp.Status = "402 Payment Required"
	}
	return resp, nil
}

// requestBodySnapshot copies an outgoing request body without consuming it.
// norma builds every model request from a *bytes.Reader, so net/http populates
// GetBody and the copy has no effect on what gets sent.
func requestBodySnapshot(req *http.Request) string {
	if req.GetBody == nil {
		return ""
	}
	rc, err := req.GetBody()
	if err != nil {
		return ""
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return ""
	}
	return string(b)
}

func quotaAwareHTTPClient(proxy, sessionHeaderKey string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		transport.Proxy = nil // empty = direct connection, without falling back to the HTTP_PROXY/HTTPS_PROXY environment variables
	} else {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("llm: invalid proxy %q: %w", proxy, err)
		}
		switch proxyURL.Scheme {
		case "http", "https", "socks5":
		case "":
			return nil, fmt.Errorf("llm: proxy %q missing scheme (use http://, https:// or socks5://)", proxy)
		default:
			return nil, fmt.Errorf("llm: unsupported proxy scheme %q (use http, https or socks5)", proxyURL.Scheme)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{Transport: quotaAwareTransport{base: transport, sessionHeaderKey: strings.TrimSpace(sessionHeaderKey)}}, nil
}

// logTestConnection prints the raw HTTP status code(s) and response body of a
// connection test to the server log, so "click test" leaves a diagnosable trail of
// exactly what the gateway returned — 401 bodies, quota text, empty frames — not
// just the collapsed ok/err the UI shows. Bodies are clipped to keep a chatty
// SSE stream from flooding the log.
func logTestConnection(c Config, capt *llmrec.Capture) {
	attempts := capt.Attempts()
	if len(attempts) == 0 {
		log.Printf("[llm-test] %s / %s @ %s -- no HTTP request was issued at all (configuration parsing or connecting failed outright)",
			c.Provider(), c.Model, c.BaseURL)
		return
	}
	for i, a := range attempts {
		log.Printf("[llm-test] %s / %s @ %s -- attempt %d/%d HTTP %d\nresponse body: %s",
			c.Provider(), c.Model, c.BaseURL, i+1, len(attempts), a.Status, clipBody(a.Body))
	}
}

// clipBody trims a wire body for logging. 4K is plenty to show an error JSON or
// the head of an SSE stream while bounding a runaway response.
func clipBody(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(empty)"
	}
	const max = 4096
	if len(s) > max {
		return s[:max] + fmt.Sprintf("...(truncated, %d bytes in total)", len(s))
	}
	return s
}

// TestConnection makes a minimal real completion to verify the provider/model/
// endpoint/key actually work. Returns the round-trip latency and the model's
// reply text.
func TestConnection(ctx context.Context, c Config) (time.Duration, string, error) {
	prov, err := c.NewProvider()
	if err != nil {
		return 0, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// Capture the raw wire messages: what a connection test most needs to show is what the gateway
	// actually returned (the status code + the response body),
	// which is lost once norma decodes the response into StreamEvents. quotaAwareTransport finds this
	// Capture on the context and fills in the status code and body of every HTTP attempt.
	ctx, capt := llmrec.NewCapture(ctx)
	defer logTestConnection(c, capt)
	// A connection test is a one-shot path that does not go through agentcore's session loop, so nobody
	// attaches a session id to the context. For an endpoint configured with SessionHeaderKey (such as
	// opencode zen, which requires the x-opencode-session header and returns 400 MissingSessionID without
	// it) that produces the gap "chat works but clicking test returns 400". A one-off random session id is
	// attached here so the test and a real conversation share the same header logic; an endpoint without
	// SessionHeaderKey does not read it, so there is no side effect.
	ctx = transcript.WithSessionID(ctx, "conntest-"+transcript.NewSessionID())
	start := time.Now()
	// MaxTokens must be generous: a reasoning model (such as deepseek-v4-pro) produces a long stretch of
	// thinking before giving an answer (measured at ~2900 tokens even for a single "ping"). With only 32,
	// the model would still be "thinking" when it hits the output cap (finish=length), get truncated, and
	// although the connection test would still pass (err=nil) it would display as a mess of
	// "interrupted/length/resume". Give it enough budget to emit OK cleanly (finish=stop).
	// EscalateMaxTokens stays false: do not raise the cap and retry on truncation, which avoids burning through a resume loop.
	reply, err := agentcore.Run(ctx, agentcore.Options{
		Provider:       prov,
		SystemPrompt:   []string{"You are a connection test. Output exactly the two characters OK, with no thinking, no explanation and nothing else."},
		PermissionMode: acperm.ModeBypass,
		MaxTurns:       1,
		MaxTokens:      8192,
		NonStreaming:   !c.Stream, // run the connection test in this profile's real send/receive mode
	}, "ping")
	lat := time.Since(start)
	if err != nil {
		return lat, "", err
	}
	// err==nil is not enough: a request going through while the model emits not a single character really
	// does happen (thinking burns the budget, the body is swallowed by a safety policy, a compatibility
	// layer drops content). Such a configuration simply "does not reply" in a conversation while the test
	// reports success -- exactly the gap this is meant to eliminate. No visible body always counts as a failure.
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return lat, "", fmt.Errorf("the model returned no content (the request went through but no text came back)")
	}
	return lat, reply, nil
}
