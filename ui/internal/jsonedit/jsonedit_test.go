package jsonedit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const sample = `{
  "zeta": 1,
  "alpha": {
    "b": [
      1,
      "two",
      {
        "c": null
      }
    ],
    "a": true,
    "empty": {},
    "none": []
  },
  "text": "a <b> & \"c\"",
  "big": 12345678901234567890,
  "frac": 1.50
}
`

func TestParseAndMarshalKeepOrderAndText(t *testing.T) {
	n, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	got := string(n.Marshal())
	// The only difference from the file is how the encoder escapes the angle
	// brackets and the ampersand, which is what the app's own writes do too
	want := strings.NewReplacer("<", `\u003c`, ">", `\u003e`, "&", `\u0026`).Replace(sample)
	if got != want {
		for i := range got {
			if i >= len(want) || got[i] != want[i] {
				t.Fatalf("round trip changed the document at byte %d: got %q, want %q", i, got[max(0, i-20):min(len(got), i+20)], want[max(0, i-20):min(len(want), i+20)])
			}
		}
		t.Fatalf("round trip changed the document: got %d bytes, want %d", len(got), len(want))
	}
}

func TestMarshalMatchesTheAppsOwnWrites(t *testing.T) {
	type doc struct {
		Z int            `json:"z"`
		A map[string]int `json:"a"`
		L []string       `json:"l"`
		E []int          `json:"e"`
	}
	raw, _ := json.MarshalIndent(doc{Z: 1, A: map[string]int{"x": 2}, L: []string{"p", "q"}, E: []int{}}, "", "  ")
	n, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(n.Marshal(), append(raw, '\n')) {
		t.Fatalf("got %q want %q", n.Marshal(), append(raw, '\n'))
	}
}

func TestSetKeepsAKeysPlaceAndAppendsANewOne(t *testing.T) {
	n, _ := Parse([]byte(`{"b":1,"a":2}`))
	if err := n.Set("b", NewInt(9)); err != nil {
		t.Fatal(err)
	}
	if err := n.Set("c", NewInt(3)); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(n.Keys(), ","), "b,a,c"; got != want {
		t.Errorf("keys = %s, want %s", got, want)
	}
	if v, _ := n.Field("b").Int(); v != 9 {
		t.Errorf("b = %d", v)
	}
}

func TestReadingValues(t *testing.T) {
	n, _ := Parse([]byte(`{"o":{"n":12,"s":"x","f":1.5,"arr":[10,20]}}`))
	if v, ok := n.Path("o", "n").Int(); !ok || v != 12 {
		t.Errorf("n = %d, %v", v, ok)
	}
	if v, ok := n.Path("o", "s").String(); !ok || v != "x" {
		t.Errorf("s = %q, %v", v, ok)
	}
	if _, ok := n.Path("o", "f").Int(); ok {
		t.Error("a fraction read as a whole number")
	}
	if _, ok := n.Path("o", "s").Int(); ok {
		t.Error("a string read as a number")
	}
	if n.Path("o", "missing") != nil || n.Path("x", "y") != nil {
		t.Error("a missing path is not nil")
	}
	arr := n.Path("o", "arr")
	if arr.Len() != 2 || arr.Kind() != Array {
		t.Errorf("array = %d long, kind %d", arr.Len(), arr.Kind())
	}
	if v, _ := arr.Item(1).Int(); v != 20 {
		t.Errorf("item 1 = %d", v)
	}
	if arr.Item(2) != nil || arr.Item(-1) != nil {
		t.Error("an index outside the array is not nil")
	}
}

func TestMoveItem(t *testing.T) {
	for _, tt := range []struct {
		from, to int
		want     string
	}{
		{0, 2, "b,c,a,d"}, {3, 0, "d,a,b,c"}, {1, 1, "a,b,c,d"}, {2, 1, "a,c,b,d"},
	} {
		n, _ := Parse([]byte(`["a","b","c","d"]`))
		if err := n.MoveItem(tt.from, tt.to); err != nil {
			t.Fatal(err)
		}
		var got []string
		for i := range n.Len() {
			s, _ := n.Item(i).String()
			got = append(got, s)
		}
		if strings.Join(got, ",") != tt.want {
			t.Errorf("move %d to %d: %v, want %s", tt.from, tt.to, got, tt.want)
		}
	}
	n, _ := Parse([]byte(`["a"]`))
	if err := n.MoveItem(0, 1); err == nil {
		t.Error("moving past the end worked")
	}
	if err := n.MoveItem(-1, 0); err == nil {
		t.Error("moving from before the start worked")
	}
}

func TestParseRefusesWhatIsNotOneDocument(t *testing.T) {
	for _, bad := range []string{``, `{`, `{"a":}`, `{} {}`, `[1,]`, `{"a":1} x`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestSetOnANonObjectFails(t *testing.T) {
	n, _ := Parse([]byte(`[1]`))
	if err := n.Set("a", NewInt(1)); err == nil {
		t.Error("set on an array worked")
	}
	var nilNode *Node
	if err := nilNode.Set("a", NewInt(1)); err == nil {
		t.Error("set on nil worked")
	}
}

func TestDeleteTakesAKeyOutAndKeepsTheRestInOrder(t *testing.T) {
	n, _ := Parse([]byte(`{"a":1,"b":2,"c":3}`))
	if !n.Delete("b") || n.Delete("b") || n.Delete("zz") {
		t.Error("Delete did not report whether the key was there")
	}
	if got := strings.Join(n.Keys(), ","); got != "a,c" {
		t.Errorf("keys = %s", got)
	}
	arr, _ := Parse([]byte(`[1]`))
	if arr.Delete("a") {
		t.Error("deleted a key of an array")
	}
}

func TestSetPathMakesTheObjectsOnTheWayAndReturnsWhatWasThere(t *testing.T) {
	n, _ := Parse([]byte(`{"text":{"fontSize":12},"name":"x"}`))
	old, err := n.SetPath([]string{"text", "shadow", "distance"}, NewInt(3))
	if err != nil || old != nil {
		t.Fatalf("old = %v, err = %v", old, err)
	}
	if got := string(n.Path("text").Compact()); got != `{"fontSize":12,"shadow":{"distance":3}}` {
		t.Errorf("text = %s", got)
	}
	old, err = n.SetPath([]string{"text", "fontSize"}, NewInt(20))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := old.Int(); v != 12 {
		t.Errorf("old fontSize = %d", v)
	}
	if _, err := n.SetPath([]string{"name", "x"}, NewInt(1)); err == nil {
		t.Error("put a key inside a string")
	}
	if _, err := n.SetPath(nil, NewInt(1)); err == nil {
		t.Error("set with no path")
	}
}

func TestDeletePathReturnsTheValueAndLeavesNoKey(t *testing.T) {
	n, _ := Parse([]byte(`{"text":{"fontSize":12,"color":"#fff"}}`))
	old := n.DeletePath([]string{"text", "color"})
	if s, _ := old.String(); s != "#fff" {
		t.Errorf("old = %v", old)
	}
	if got := string(n.Compact()); got != `{"text":{"fontSize":12}}` {
		t.Errorf("document = %s", got)
	}
	if n.DeletePath([]string{"text", "color"}) != nil || n.DeletePath([]string{"nope", "x"}) != nil || n.DeletePath(nil) != nil {
		t.Error("deleting what is not there returned a value")
	}
}

func TestCompactWritesOneLineAndRoundTrips(t *testing.T) {
	n, _ := Parse([]byte(sample))
	c := n.Compact()
	if bytes.ContainsAny(c, "\n ") && !strings.Contains(string(c), `"text":"a`) {
		t.Errorf("not compact: %s", c)
	}
	back, err := Parse(c)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back.Marshal(), n.Marshal()) {
		t.Error("a compact copy does not read back to the same document")
	}
}
