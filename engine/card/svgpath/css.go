package svgpath

import (
	"fmt"
	"strings"
)

// decls is a set of CSS property declarations, from a style attribute or a
// rule's block
type decls map[string]string

// parseDecls reads a list of name and value declarations separated by semicolons
func parseDecls(s string) decls {
	out := decls{}
	for _, d := range strings.Split(s, ";") {
		name, val, ok := strings.Cut(d, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		val = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(val), "!important"))
		if name != "" && val != "" {
			out[name] = val
		}
	}
	return out
}

// rule is one stylesheet rule, a selector list and its declarations
type rule struct {
	selectors []selector
	decls     decls
}

// selector is a class selector (".cls-1"), a type selector ("path") or the
// universal one, the only forms the icons' stylesheets use
type selector struct {
	class string // set for a class selector
	tag   string // set for a type selector, empty for "*" and class selectors
}

func (s selector) matches(n *node, classes []string) bool {
	switch {
	case s.class != "":
		for _, c := range classes {
			if c == s.class {
				return true
			}
		}
		return false
	case s.tag != "":
		return n.name == s.tag
	}
	return true
}

// parseStylesheet reads rules of the form "sel, sel { decls }". A selector
// beyond a class, a type or "*", and any at-rule, is reported as unsupported,
// since skipping it could change what the icon looks like
func parseStylesheet(css string) ([]rule, error) {
	for {
		start := strings.Index(css, "/*")
		if start < 0 {
			break
		}
		end := strings.Index(css[start+2:], "*/")
		if end < 0 {
			css = css[:start]
			break
		}
		css = css[:start] + css[start+2+end+2:]
	}
	var rules []rule
	for _, block := range strings.Split(css, "}") {
		sels, body, ok := strings.Cut(block, "{")
		if !ok {
			continue
		}
		sels = strings.TrimSpace(sels)
		if strings.HasPrefix(sels, "@") {
			return nil, fmt.Errorf("%w: CSS at-rule %q", ErrUnsupported, sels)
		}
		r := rule{decls: parseDecls(body)}
		for _, s := range strings.Split(sels, ",") {
			s = strings.TrimSpace(s)
			switch {
			case s == "*":
				r.selectors = append(r.selectors, selector{})
			case strings.HasPrefix(s, ".") && validIdent(s[1:]):
				r.selectors = append(r.selectors, selector{class: s[1:]})
			case validIdent(s):
				r.selectors = append(r.selectors, selector{tag: s})
			default:
				return nil, fmt.Errorf("%w: CSS selector %q", ErrUnsupported, s)
			}
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func validIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
