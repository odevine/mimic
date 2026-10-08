package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/odevine/mimic/ui/internal/mcpstate"
)

func TestParseArgumentsReadsJSONValuesAndKeepsTheRestAsText(t *testing.T) {
	got, err := parseArguments([]string{"selector=#results .result", "limit=3", "visible=true", "text=hello world", "ids=[1,2]", "empty="}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"selector": "#results .result", "limit": float64(3), "visible": true, "text": "hello world", "ids": []any{float64(1), float64(2)}, "empty": ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("arguments = %#v, want %#v", got, want)
	}
}

func TestParseArgumentsFromJSONObject(t *testing.T) {
	got, err := parseArguments(nil, `{"code":"return 1","timeout_ms":500}`)
	if err != nil || got["code"] != "return 1" || got["timeout_ms"] != float64(500) {
		t.Errorf("arguments = %v, %v", got, err)
	}
	if _, err := parseArguments([]string{"a=1"}, `{"b":2}`); err == nil {
		t.Error("both forms at once should be refused")
	}
	if _, err := parseArguments(nil, `[1]`); err == nil {
		t.Error("a JSON array is not an argument object")
	}
	if _, err := parseArguments([]string{"nokey"}, ""); err == nil {
		t.Error("a word with no = should be refused")
	}
}

// fakeMCP answers like the Wails endpoint: stateless JSON-RPC behind a bearer token
func fakeMCP(t *testing.T, token string, handle func(method string, params json.RawMessage) any) mcpstate.State {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": handle(req.Method, req.Params)})
	}))
	t.Cleanup(srv.Close)
	return mcpstate.State{URL: srv.URL, Token: token}
}

func text(s string, isError bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": s}}, "isError": isError}
}

func TestCallToolSendsTheNameArgumentsAndToken(t *testing.T) {
	var got struct {
		Name      string
		Arguments map[string]any
	}
	state := fakeMCP(t, "tok", func(method string, params json.RawMessage) any {
		if method != "tools/call" {
			t.Errorf("method = %q", method)
		}
		json.Unmarshal(params, &got)
		return text("hello", false)
	})
	out, isErr, err := callTool(state, "dom_query", map[string]any{"selector": "#x"}, 5e9)
	if err != nil || isErr || out != "hello" {
		t.Fatalf("callTool = %q, %v, %v", out, isErr, err)
	}
	if got.Name != "dom_query" || got.Arguments["selector"] != "#x" {
		t.Errorf("the endpoint received %+v", got)
	}
}

func TestCallToolReportsAToolError(t *testing.T) {
	state := fakeMCP(t, "tok", func(string, json.RawMessage) any { return text("no such element", true) })
	out, isErr, err := callTool(state, "dom_query", nil, 5e9)
	if err != nil || !isErr || out != "no such element" {
		t.Errorf("callTool = %q, %v, %v", out, isErr, err)
	}
}

func TestACallWithTheWrongTokenFails(t *testing.T) {
	state := fakeMCP(t, "right", func(string, json.RawMessage) any { return text("", false) })
	state.Token = "wrong"
	if _, _, err := callTool(state, "x", nil, 5e9); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v, want a 401", err)
	}
}

func TestToolsListsNamesDescriptionsAndArguments(t *testing.T) {
	state := fakeMCP(t, "tok", func(method string, _ json.RawMessage) any {
		return map[string]any{"tools": []map[string]any{{
			"name": "dom_query", "description": "Find elements",
			"inputSchema": map[string]any{
				"properties": map[string]any{"selector": map[string]any{"type": "string", "description": "CSS selector"}},
				"required":   []string{"selector"},
			},
		}}}
	})
	var out bytes.Buffer
	if err := printTools(state, 5e9, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dom_query", "Find elements", "selector string (required): CSS selector"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("tools output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestRunWithNothingRunningSaysHowToStartOne(t *testing.T) {
	// The module's bin folder is where the state file lives, and a test must not
	// depend on a developer's running build, so it works in a copy of the layout
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/odevine/mimic/ui\n"), 0o644)
	wd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(wd) })
	os.Chdir(root)
	err := run([]string{"dom_query"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "dev:mcp") {
		t.Errorf("err = %v, want one pointing at dev:mcp", err)
	}
}
