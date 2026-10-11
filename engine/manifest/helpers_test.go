package manifest

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

// gone, given to set, removes the key instead of setting it
var gone = new(int)

func smallJSON(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/small.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// edit decodes the small manifest into a tree, lets f change it, and encodes it again
func edit(t *testing.T, f func(root map[string]any)) []byte {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(smallJSON(t), &root); err != nil {
		t.Fatal(err)
	}
	f(root)
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// set puts value at a path like layouts.normal.stack[1].part, or removes the key
// when value is gone
func set(t *testing.T, root map[string]any, path string, value any) {
	t.Helper()
	segs := strings.Split(path, ".")
	var cur any = root
	for i, seg := range segs {
		name, idx, hasIdx := splitIndex(t, seg)
		last := i == len(segs)-1
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("set %s: %q is not inside an object", path, seg)
		}
		if hasIdx {
			list, ok := m[name].([]any)
			if !ok || idx >= len(list) {
				t.Fatalf("set %s: no item %s", path, seg)
			}
			if last {
				list[idx] = value
				return
			}
			cur = list[idx]
			continue
		}
		if last {
			if value == gone {
				delete(m, name)
			} else {
				m[name] = value
			}
			return
		}
		next, ok := m[name]
		if !ok {
			next = map[string]any{}
			m[name] = next
		}
		cur = next
	}
}

func splitIndex(t *testing.T, seg string) (string, int, bool) {
	t.Helper()
	open := strings.IndexByte(seg, '[')
	if open < 0 {
		return seg, 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(seg[open+1:], "]"))
	if err != nil {
		t.Fatalf("bad index in %q", seg)
	}
	return seg[:open], n, true
}

// find returns the diagnostic at path, if there is one
func find(ds Diagnostics, path string) (Diagnostic, bool) {
	for _, d := range ds {
		if d.Path == path {
			return d, true
		}
	}
	return Diagnostic{}, false
}
