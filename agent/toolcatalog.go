package agent

import (
	"context"
	"encoding/json"

	actool "github.com/Autumn-27/norma/tool"
)

// This file turns the "builtin tools" from pure code into a catalog that can be enumerated and overridden by the DB:
//   - BuiltinToolSeeds(): expands the three executing agents' builtin tool sets into seed records (key +
//     description + parameter schema + the agents they are bound to by default), for the server to seed idempotently into the tools table at startup.
//   - The ToolResolve hook: at runtime it applies the tools rows in the DB to the assembled tools --
//     "filter by agent + override the description/schema + inject parameter defaults". The key and handler stay in code; the DB only changes "the prose and the defaults".
// The handler (the Call behaviour) always comes from code -- the DB cannot change it, only the description the model sees and the default arguments.

// ToolSeed is a seedable snapshot of one builtin tool: key is CoreTool.Name() (hard-bound to the
// handler and read-only in the UI), Desc/Schema come from the tool definition in code, and Agents is which agents the code gives it to by default.
type ToolSeed struct {
	Key    string         // = CoreTool.Name(), the primary key, immutable
	Desc   string         // the top-level description (overridable in the UI)
	Schema map[string]any // the parameter JSON Schema (the structure is read-only; description/default can be changed in the UI)
	Agents []string       // the agent keys it is bound to by default (worker/planner/mainagent)
}

// builtinToolsByAgent builds each executing agent's domain tool set with a "read-only shell" ToolSet
// (nil stores). A tool constructor only puts closures into its Spec and never dereferences a store at
// construction time, so nil is safe here --
// these tools are used only to read Name()/Description()/InputSchema() and are never Called.
//
// It deliberately [excludes] the SDK's generic tools actool.DefaultTools() (Read/Write/Edit/MultiEdit/
// LS/Glob/Grep/Bash): every agent always has them, there is no "which agent gets it" trade-off, and most
// of their documentation lives in Prompt() (this table only overrides Description(), which would be
// half an override and misleading). Not seeded -> no DB row -> ToolResolve passes them through
// unchanged, behaving exactly as before. Only artex's own domain tools enter the table and become manageable.
func builtinToolsByAgent() map[string][]actool.CoreTool {
	ts := NewToolSet(nil, "")
	return map[string][]actool.CoreTool{
		"mainagent": ts.MainAgentTools(),
		"planner":   ts.PlannerTools(),
		"worker":    append(ts.WorkerTools(), httpRequestTool(nil)),
		// goals (the goal decomposer) is bound to set_goals + set_constraints by default: they are what
		// write the decomposed goals and extracted operating constraints into the database. It shares the same managed tools as mainagent, whose description/schema can be edited in the web UI and ticked per agent.
		"goals": {ts.setGoals(), ts.setConstraints()},
		// auto is bound by default to the finding reporting + asset management tools; the other domain tools can be ticked in the UI as needed.
		// A new database gets them from this seed; an old one is migrated by seedAutoDefaultBindings.
		"auto": {ts.addFinding(), ts.insertAssets(), ts.addCompanyScope(), ts.listAssets(), ts.listCompanies()},
		// pentest (the standalone penetration agent) is bound by default to: query assets / insert assets / report findings / query findings / query companies.
		// A new database gets them from this seed; an old one is migrated by seedPentestDefaultBindings.
		"pentest": {ts.listAssets(), ts.insertAssets(), ts.addFinding(), ts.listFindings(), ts.listCompanies()},
	}
}

// defaultUnbound: these system tools still enter the catalog (visible in the web UI and tickable per
// agent by hand) but are [bound to no agent] by default -- ToolResolve drops a tool with an empty binding for every agent, so it must be opted into explicitly.
// The reason they remain in some agent's base tool set (such as goal_met in PlannerTools) is twofold:
// it lets the seed construct them to obtain their desc/schema, and once a user binds one back it must be in base at runtime for ToolResolve to keep it.
//
// goal_met: it bypasses proving goals one by one and declares [the whole task complete] globally, which
// carries a lot of weight and a risk of misjudgement, and it duplicates "prove_goal marking the last goal -> automatic wrap-up", so no agent gets it by default and it is bound by hand when needed.
var defaultUnbound = map[string]bool{"goal_met": true, "http_request": true}

// BuiltinToolSeeds deduplicates and merges each agent's builtin tool set into a seed list: a tool with
// the same name (such as list_assets, which several agents have) becomes one entry whose Agents is the union; a tool in defaultUnbound is forced to an empty binding.
func BuiltinToolSeeds() []ToolSeed {
	byAgent := builtinToolsByAgent()
	order := []string{"mainagent", "goals", "planner", "worker", "auto", "pentest"}

	type acc struct {
		tool   actool.CoreTool
		agents []string
	}
	m := map[string]*acc{}
	var keys []string
	for _, ak := range order {
		for _, t := range byAgent[ak] {
			a, ok := m[t.Name()]
			if !ok {
				a = &acc{tool: t}
				m[t.Name()] = a
				keys = append(keys, t.Name())
			}
			a.agents = append(a.agents, ak)
		}
	}

	out := make([]ToolSeed, 0, len(keys))
	for _, k := range keys {
		a := m[k]
		agents := a.agents
		if defaultUnbound[k] {
			agents = []string{} // enters the catalog and can be bound by hand, but is given to no agent by default (stored as [] rather than null, like the other tools)
		}
		out = append(out, ToolSeed{
			Key:    k,
			Desc:   a.tool.Description(),
			Schema: a.tool.InputSchema(),
			Agents: agents,
		})
	}
	return out
}

// ToolResolve, if set, post-processes an agent's fully-assembled tool list against
// the DB tools table: it drops tools not bound to this agent (or globally disabled)
// and wraps the rest so the model sees the DB-overridden description/schema and
// default arguments get injected. Tools with no matching DB row (MCP/skill/host tools like
// traffic) pass through untouched. nil = tools unchanged. Wired in server/assembly.go.
var ToolResolve func(ctx context.Context, agentKey string, tools []actool.CoreTool) []actool.CoreTool

// DecorateTool wraps t so Description()/InputSchema() report the DB overrides and
// Call() injects scalar parameter defaults (from schema's "default" props) whenever
// the model omitted them. Name/Prompt/permission/scheduler flags delegate to t, so
// the tool's identity and handler are unchanged. Empty desc/schema fall back to t's.
func DecorateTool(t actool.CoreTool, desc string, schema map[string]any) actool.CoreTool {
	if desc == "" {
		desc = t.Description()
	}
	if len(schema) == 0 {
		schema = t.InputSchema()
	}
	return &overriddenTool{CoreTool: t, desc: desc, schema: schema}
}

// overriddenTool is a CoreTool decorator: it embeds the original (so all behavioral
// methods — Prompt/IsReadOnly/IsConcurrencySafe/CheckPermissions/Name — delegate)
// and overrides only the model-facing description/schema plus default injection.
type overriddenTool struct {
	actool.CoreTool
	desc   string
	schema map[string]any
}

func (o *overriddenTool) Description() string         { return o.desc }
func (o *overriddenTool) InputSchema() map[string]any { return o.schema }

func (o *overriddenTool) Call(ctx context.Context, in json.RawMessage, tc *actool.ToolContext) (actool.Result, error) {
	return o.CoreTool.Call(ctx, injectDefaults(in, o.schema), tc)
}

// injectDefaults fills scalar parameter defaults declared in the (possibly edited)
// schema into the input JSON whenever the model omitted the field or left it empty/
// null. Structure (names/types/required) is untouched — only the defaults are merged in.
func injectDefaults(in json.RawMessage, schema map[string]any) json.RawMessage {
	defs := scalarDefaults(schema)
	if len(defs) == 0 {
		return in
	}
	m := map[string]json.RawMessage{}
	if len(in) > 0 {
		if err := json.Unmarshal(in, &m); err != nil {
			return in // non-object input: don't touch it
		}
	}
	changed := false
	for k, dv := range defs {
		if cur, ok := m[k]; !ok || isEmptyJSON(cur) {
			m[k] = dv
			changed = true
		}
	}
	if !changed {
		return in
	}
	b, err := json.Marshal(m)
	if err != nil {
		return in
	}
	return b
}

// scalarDefaults extracts properties[k]["default"] for scalar params (string/
// integer/number/boolean). Array/object defaults are skipped: merging them is
// ambiguous and not worth the surprise.
func scalarDefaults(schema map[string]any) map[string]json.RawMessage {
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return nil
	}
	out := map[string]json.RawMessage{}
	for name, raw := range props {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		dv, ok := p["default"]
		if !ok || dv == nil {
			continue
		}
		switch p["type"] {
		case "string", "integer", "number", "boolean":
			if b, err := json.Marshal(dv); err == nil {
				out[name] = b
			}
		}
	}
	return out
}

func isEmptyJSON(raw json.RawMessage) bool {
	s := string(raw)
	return s == "null" || s == `""`
}
