// Package jsonedit changes a JSON document in place and writes it back in the
// order and layout it had. Decoding into a map and encoding again sorts the keys,
// which would scramble a file a person also edits by hand, so a document here is a
// tree that keeps each object's keys where they were
package jsonedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Kind is what a node holds
type Kind uint8

const (
	// Scalar is a string, a number, a boolean or null, kept as the text it was
	Scalar Kind = iota
	Object
	Array
)

// Node is one value of a document
type Node struct {
	kind   Kind
	raw    []byte
	keys   []string
	fields map[string]*Node
	items  []*Node
}

// Parse reads one JSON value and nothing after it
func Parse(data []byte) (*Node, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	n, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("jsonedit: more than one value")
	}
	return n, nil
}

func parseValue(dec *json.Decoder) (*Node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			n := &Node{kind: Object, fields: map[string]*Node{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("jsonedit: %v is not a key", kt)
				}
				v, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				if _, dup := n.fields[key]; !dup {
					n.keys = append(n.keys, key)
				}
				n.fields[key] = v
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return n, nil
		case '[':
			n := &Node{kind: Array}
			for dec.More() {
				v, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				n.items = append(n.items, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return n, nil
		}
		return nil, fmt.Errorf("jsonedit: unexpected %v", t)
	case json.Number:
		return &Node{raw: []byte(t.String())}, nil
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		return &Node{raw: raw}, nil
	}
}

// Kind says what the node holds
func (n *Node) Kind() Kind { return n.kind }

// Field is the value under a key of an object, or nil when there is none
func (n *Node) Field(name string) *Node {
	if n == nil || n.kind != Object {
		return nil
	}
	return n.fields[name]
}

// Path follows keys down through objects, and returns nil when any is missing
func (n *Node) Path(names ...string) *Node {
	for _, name := range names {
		if n = n.Field(name); n == nil {
			return nil
		}
	}
	return n
}

// Keys lists an object's keys in the order they are written
func (n *Node) Keys() []string {
	if n == nil || n.kind != Object {
		return nil
	}
	return append([]string(nil), n.keys...)
}

// Len is the number of items in an array, and the number of keys in an object
func (n *Node) Len() int {
	switch {
	case n == nil:
		return 0
	case n.kind == Array:
		return len(n.items)
	case n.kind == Object:
		return len(n.keys)
	}
	return 0
}

// Item is the value at an index of an array, or nil when there is none
func (n *Node) Item(i int) *Node {
	if n == nil || n.kind != Array || i < 0 || i >= len(n.items) {
		return nil
	}
	return n.items[i]
}

// Int reads a node that holds a whole number
func (n *Node) Int() (int, bool) {
	if n == nil || n.kind != Scalar {
		return 0, false
	}
	var v int
	if err := json.Unmarshal(n.raw, &v); err != nil {
		return 0, false
	}
	return v, true
}

// String reads a node that holds a string
func (n *Node) String() (string, bool) {
	if n == nil || n.kind != Scalar {
		return "", false
	}
	var v string
	if err := json.Unmarshal(n.raw, &v); err != nil {
		return "", false
	}
	return v, true
}

// NewInt is a node that holds a whole number
func NewInt(v int) *Node { return &Node{raw: []byte(fmt.Sprint(v))} }

// NewObject is an object with no keys
func NewObject() *Node { return &Node{kind: Object, fields: map[string]*Node{}} }

// Set puts a value under a key of an object. A key already there keeps its place,
// and a new one goes last
func (n *Node) Set(name string, v *Node) error {
	if n == nil || n.kind != Object {
		return errors.New("jsonedit: not an object")
	}
	if _, ok := n.fields[name]; !ok {
		n.keys = append(n.keys, name)
	}
	n.fields[name] = v
	return nil
}

// Delete takes a key out of an object and reports whether it was there
func (n *Node) Delete(name string) bool {
	if n == nil || n.kind != Object {
		return false
	}
	if _, ok := n.fields[name]; !ok {
		return false
	}
	delete(n.fields, name)
	for i, k := range n.keys {
		if k == name {
			n.keys = append(n.keys[:i], n.keys[i+1:]...)
			break
		}
	}
	return true
}

// SetPath puts a value at a path of keys below an object, making an object for each
// key that is missing on the way, and returns the value that was there, or nil when
// there was none. A key on the way that holds something other than an object is an
// error
func (n *Node) SetPath(path []string, v *Node) (*Node, error) {
	if len(path) == 0 {
		return nil, errors.New("jsonedit: no path")
	}
	at := n
	for _, name := range path[:len(path)-1] {
		next := at.Field(name)
		switch {
		case at == nil || at.kind != Object:
			return nil, errors.New("jsonedit: not an object")
		case next == nil:
			next = NewObject()
			_ = at.Set(name, next)
		case next.kind != Object:
			return nil, fmt.Errorf("jsonedit: %q holds no object to put a value in", name)
		}
		at = next
	}
	last := path[len(path)-1]
	old := at.Field(last)
	if err := at.Set(last, v); err != nil {
		return nil, err
	}
	return old, nil
}

// DeletePath takes out the value at a path of keys and returns it, or nil when there
// was none
func (n *Node) DeletePath(path []string) *Node {
	if len(path) == 0 {
		return nil
	}
	parent := n.Path(path[:len(path)-1]...)
	old := parent.Field(path[len(path)-1])
	if old != nil {
		parent.Delete(path[len(path)-1])
	}
	return old
}

// MoveItem takes the item at index from out of an array and puts it back so that
// it is at index to
func (n *Node) MoveItem(from, to int) error {
	if n == nil || n.kind != Array || from < 0 || from >= len(n.items) || to < 0 || to >= len(n.items) {
		return errors.New("jsonedit: no such place in the array")
	}
	it := n.items[from]
	n.items = append(n.items[:from], n.items[from+1:]...)
	n.items = append(n.items[:to], append([]*Node{it}, n.items[to:]...)...)
	return nil
}

// Compact writes the value on one line with no spaces, which is how a value is sent
// to be put back
func (n *Node) Compact() []byte {
	var b bytes.Buffer
	n.compact(&b)
	return b.Bytes()
}

func (n *Node) compact(b *bytes.Buffer) {
	switch n.kind {
	case Scalar:
		b.Write(n.raw)
	case Object:
		b.WriteByte('{')
		for i, k := range n.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			key, _ := json.Marshal(k)
			b.Write(key)
			b.WriteByte(':')
			n.fields[k].compact(b)
		}
		b.WriteByte('}')
	case Array:
		b.WriteByte('[')
		for i, it := range n.items {
			if i > 0 {
				b.WriteByte(',')
			}
			it.compact(b)
		}
		b.WriteByte(']')
	}
}

// Marshal writes the document with two spaces of indent, the layout the app writes
// its own files in, and a newline after it
func (n *Node) Marshal() []byte {
	var b bytes.Buffer
	n.write(&b, 0)
	b.WriteByte('\n')
	return b.Bytes()
}

func (n *Node) write(b *bytes.Buffer, depth int) {
	indent := strings.Repeat("  ", depth+1)
	switch n.kind {
	case Scalar:
		b.Write(n.raw)
	case Object:
		if len(n.keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, k := range n.keys {
			b.WriteString(indent)
			key, _ := json.Marshal(k)
			b.Write(key)
			b.WriteString(": ")
			n.fields[k].write(b, depth+1)
			if i < len(n.keys)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(strings.Repeat("  ", depth) + "}")
	case Array:
		if len(n.items) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, it := range n.items {
			b.WriteString(indent)
			it.write(b, depth+1)
			if i < len(n.items)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(strings.Repeat("  ", depth) + "]")
	}
}
