// Command mcpcall sends one tool call to the MCP endpoint of a running mcp test
// build, so an agent with only a shell can inspect and drive the real window
// without accessibility access. Start the build with the dev:mcp task first.
//
//	mcpcall tools                        list the tools and what they take
//	mcpcall dom_query selector=#results  call a tool, arguments as key=value
//	mcpcall js_eval -json '{"code":"return document.title"}'
//	mcpcall stop                         stop the running build
//
// A value that reads as JSON, such as 3, true or [1,2], is sent as that JSON,
// and anything else is sent as text. The tool's text result is printed, and a
// tool that reports an error exits with status 1
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/odevine/mimic/ui/internal/mcpstate"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mcpcall:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("mcpcall", flag.ContinueOnError)
	rawJSON := fs.String("json", "", "the tool's arguments as one JSON object, instead of key=value pairs")
	timeout := fs.Duration("timeout", 60*time.Second, "how long to wait for the tool")
	// Flags may come after the tool name, so the name is taken first
	if len(args) == 0 {
		return errors.New("usage: mcpcall tools | stop | <tool> [key=value ...] [-json '{...}']")
	}
	name, rest := args[0], args[1:]
	if err := fs.Parse(rest); err != nil {
		return err
	}

	root, err := mcpstate.ModuleRoot()
	if err != nil {
		return err
	}
	state, err := mcpstate.Read(root)
	if err != nil {
		return err
	}

	switch name {
	case "stop":
		return stop(state)
	case "tools":
		return printTools(state, *timeout, out)
	}

	arguments, err := parseArguments(fs.Args(), *rawJSON)
	if err != nil {
		return err
	}
	text, isError, err := callTool(state, name, arguments, *timeout)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, text)
	if isError {
		return errors.New("the tool reported an error")
	}
	return nil
}

// parseArguments builds a tool's arguments from key=value pairs, or from a JSON
// object when one is given
func parseArguments(pairs []string, rawJSON string) (map[string]any, error) {
	args := map[string]any{}
	if rawJSON != "" {
		if len(pairs) > 0 {
			return nil, errors.New("give the arguments as -json or as key=value pairs, not both")
		}
		if err := json.Unmarshal([]byte(rawJSON), &args); err != nil {
			return nil, fmt.Errorf("-json is not a JSON object: %w", err)
		}
		return args, nil
	}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("%q is not key=value", p)
		}
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			parsed = v
		}
		// A bare word that happens to read as JSON text, like "null", stays a number,
		// bool or null on purpose. Quote it in -json to send it as text
		args[k] = parsed
	}
	return args, nil
}

// rpc sends one JSON-RPC request and returns its result. The endpoint keeps no
// session, so no handshake comes first
func rpc(state mcpstate.State, method string, params any, timeout time.Duration) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, state.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+state.Token)
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("the build at %s did not answer, so it may have stopped: %w", state.URL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the endpoint answered %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, fmt.Errorf("the endpoint's answer is not JSON: %w", err)
	}
	if reply.Error != nil {
		return nil, errors.New(reply.Error.Message)
	}
	return reply.Result, nil
}

// callTool calls one tool and returns its text, and whether it reported an error
func callTool(state mcpstate.State, name string, arguments map[string]any, timeout time.Duration) (string, bool, error) {
	result, err := rpc(state, "tools/call", map[string]any{"name": name, "arguments": arguments}, timeout)
	if err != nil {
		return "", false, err
	}
	var r struct {
		Content []struct {
			Type, Text string
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &r); err != nil {
		return "", false, err
	}
	var parts []string
	for _, c := range r.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n"), r.IsError, nil
}

func printTools(state mcpstate.State, timeout time.Duration, out io.Writer) error {
	result, err := rpc(state, "tools/list", map[string]any{}, timeout)
	if err != nil {
		return err
	}
	var list struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema struct {
				Properties map[string]struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				} `json:"properties"`
				Required []string `json:"required"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &list); err != nil {
		return err
	}
	for _, t := range list.Tools {
		fmt.Fprintf(out, "%s\n  %s\n", t.Name, t.Description)
		required := map[string]bool{}
		for _, r := range t.InputSchema.Required {
			required[r] = true
		}
		for name, p := range t.InputSchema.Properties {
			mark := ""
			if required[name] {
				mark = " (required)"
			}
			fmt.Fprintf(out, "    %s %s%s: %s\n", name, p.Type, mark, p.Description)
		}
	}
	return nil
}

// stop asks the test build to quit by signalling the testserver that runs it
func stop(state mcpstate.State) error {
	p, err := os.FindProcess(state.PID)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return p.Kill()
	}
	if err := p.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("signalling process %d: %w", state.PID, err)
	}
	return nil
}
