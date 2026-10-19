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
