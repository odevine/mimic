package svgpath

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	maxNodes = 50000
	xlinkNS  = "http://www.w3.org/1999/xlink"
)

// node is one SVG element with the attributes and children that matter here.
// Elements in other namespaces, such as an editor's metadata, are left out of
// the tree
type node struct {
	name string
	attr map[string]string
	kids []*node
	text string
}

// document is a parsed SVG file
type document struct {
	root  *node
	ids   map[string]*node
	rules []rule
}

// parseDocument reads the file into a tree. It limits nesting and the number of
// elements, since the file is network input
func parseDocument(data []byte) (*document, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	// A file that declares a charset other than UTF-8 is read as UTF-8 anyway,
	// since the path data is ASCII and decoding the rest does not matter here
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }

	doc := &document{ids: map[string]*node{}}
	var stack []*node
	count := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("svgpath: reading SVG: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != "" && t.Name.Space != svgNS {
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("svgpath: reading SVG: %w", err)
				}
				continue
			}
			if doc.root == nil && t.Name.Local != "svg" {
				return nil, fmt.Errorf("svgpath: root element is %q, not svg", t.Name.Local)
			}
			if len(stack) >= maxDepth {
				return nil, fmt.Errorf("svgpath: elements nest deeper than %d", maxDepth)
			}
			if count++; count > maxNodes {
				return nil, fmt.Errorf("svgpath: more than %d elements", maxNodes)
			}
			n := &node{name: t.Name.Local, attr: make(map[string]string, len(t.Attr))}
			for _, a := range t.Attr {
				switch a.Name.Space {
				case "":
					n.attr[a.Name.Local] = a.Value
				case xlinkNS:
					if a.Name.Local == "href" {
						n.attr["href"] = a.Value
					}
				}
			}
			if id := n.attr["id"]; id != "" {
				if _, dup := doc.ids[id]; !dup {
					doc.ids[id] = n
				}
			}
			if len(stack) == 0 {
				if doc.root != nil {
					return nil, errors.New("svgpath: more than one root element")
				}
				doc.root = n
			} else {
				parent := stack[len(stack)-1]
				parent.kids = append(parent.kids, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1].name == "style" {
				stack[len(stack)-1].text += string(t)
			}
		}
	}
	if doc.root == nil {
		return nil, errors.New("svgpath: no svg element")
	}
	var css strings.Builder
	var collect func(n *node)
	collect = func(n *node) {
		if n.name == "style" {
			if t := n.attr["type"]; t == "" || t == "text/css" {
				css.WriteString(n.text)
				css.WriteString("}\n")
			}
		}
		for _, k := range n.kids {
			collect(k)
		}
	}
	collect(doc.root)
	rules, err := parseStylesheet(css.String())
	if err != nil {
		return nil, err
	}
	doc.rules = rules
	return doc, nil
}

// classes lists an element's class names
func (n *node) classes() []string { return strings.Fields(n.attr["class"]) }

// hrefID is the id an href points at, "" for a link that is not a local
// reference
func (n *node) hrefID() string {
	h := strings.TrimSpace(n.attr["href"])
	if strings.HasPrefix(h, "#") {
		return h[1:]
	}
	return ""
}

// presentation names the properties that may be written as attributes. They sit
// beneath a stylesheet's rules, and a style attribute sits above both
var presentation = map[string]bool{
	"fill": true, "fill-opacity": true, "fill-rule": true,
	"stroke": true, "stroke-width": true, "stroke-opacity": true, "stroke-linejoin": true,
	"stroke-linecap": true, "stroke-miterlimit": true, "stroke-dasharray": true, "stroke-dashoffset": true,
	"opacity": true, "display": true, "visibility": true, "color": true,
	"stop-color": true, "stop-opacity": true, "clip-path": true, "filter": true, "mask": true,
}

// computed is the element's own property declarations, resolved from its
// attributes, then the stylesheet's rules, then its style attribute
func (d *document) computed(n *node) decls {
	out := decls{}
	for k, v := range n.attr {
		if presentation[k] {
			out[k] = strings.TrimSpace(v)
		}
	}
	if len(d.rules) > 0 {
		classes := n.classes()
		// Type and universal rules apply before class rules
		for pass := 0; pass < 2; pass++ {
			for _, r := range d.rules {
				for _, s := range r.selectors {
					if (s.class != "") == (pass == 1) && s.matches(n, classes) {
						for k, v := range r.decls {
							out[k] = v
						}
						break
					}
				}
			}
		}
	}
	if s := n.attr["style"]; s != "" {
		for k, v := range parseDecls(s) {
			out[k] = v
		}
	}
	return out
}
