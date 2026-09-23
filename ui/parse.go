package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The list formats a pasted list can be read as. Each reads everything the one
// before it does and one thing more, except csv, which is its own shape
const (
	formatNames    = "names"
	formatQuantity = "quantity"
	formatArena    = "arena"
	formatCSV      = "csv"
)

// maxListRows bounds one list so a stray paste of a huge file fails fast rather
// than queueing thousands of Scryfall lookups
const maxListRows = 1000

// maxQuantity caps a line's count, since a typo like 4000 Island is far more
// likely than a real need for that many copies of one file's worth of output
const maxQuantity = 999

// listRow is one card-producing line of a list. Query is set for a line that
// starts with ?, and Fields carries a CSV row's columns keyed by edit field
type listRow struct {
	Line   int               `json:"line"`
	Text   string            `json:"text"`
	Qty    int               `json:"qty"`
	Name   string            `json:"name,omitempty"`
	Set    string            `json:"set,omitempty"`
	Number string            `json:"number,omitempty"`
	Group  string            `json:"group,omitempty"`
	Query  string            `json:"query,omitempty"`
	Fields map[string]string `json:"fields,omitempty"`
	Custom bool              `json:"custom,omitempty"`
}

var (
	quantityRe = regexp.MustCompile(`^(\d+)x?\s+(.+)$`)
	// arenaRe matches the printing suffix of an Arena export line, with an
	// optional collector number and the *F* foil marker some exports append
	arenaRe = regexp.MustCompile(`^(.+?)\s+\(([A-Za-z0-9]{2,6})\)(?:\s+([A-Za-z0-9★†-]+))?(?:\s+\*F\*)?$`)
	// headerRe matches a decklist section header such as Sideboard or Deck:
	headerRe = regexp.MustCompile(`(?i)^(deck|main|mainboard|maindeck|sideboard|side|commander|companion|maybeboard|considering|tokens)\s*:?$`)
)

// parseList reads a pasted list into rows. format forces one reading, and an
// empty format sniffs it. It returns the format actually used, so the page can
// name it and offer the others
func parseList(text, format string) ([]listRow, string, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	if format == "" {
		format = sniffFormat(text)
	}
	var rows []listRow
	var err error
	switch format {
	case formatCSV:
		rows, err = parseCSV(text)
	case formatNames, formatQuantity, formatArena:
		rows = parseLines(text, format)
	default:
		return nil, "", fmt.Errorf("unknown list format %q", format)
	}
	if err != nil {
		return nil, format, err
	}
	if len(rows) > maxListRows {
		return nil, format, fmt.Errorf("the list has %d cards, more than the %d one run takes", len(rows), maxListRows)
	}
	return rows, format, nil
}

// sniffFormat picks the richest reading the text needs: csv when the first line
// is a header naming a known column, arena when any line carries a set in
// parentheses, quantity when any line starts with a count, and names otherwise
func sniffFormat(text string) string {
	lines := contentLines(text)
	if len(lines) == 0 {
		return formatNames
	}
	if strings.Contains(lines[0], ",") {
		if cols, err := csv.NewReader(strings.NewReader(lines[0])).Read(); err == nil && csvColumns(cols) != nil {
			return formatCSV
		}
	}
	format := formatNames
	for _, l := range lines {
		if strings.HasPrefix(l, "?") || headerRe.MatchString(l) {
			continue
		}
		body := l
		if m := quantityRe.FindStringSubmatch(l); m != nil {
			format = formatQuantity
			body = m[2]
		}
		if arenaRe.MatchString(body) {
			return formatArena
		}
	}
	return format
}

// contentLines returns the trimmed lines that are not blank or comments
func contentLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "//") && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}

// parseLines reads one card per line. A section header sets the group for the
// lines below it, and Arena's About and Name metadata lines are skipped
func parseLines(text, format string) []listRow {
	var rows []listRow
	group := ""
	about := false
	for i, raw := range strings.Split(text, "\n") {
		l := strings.TrimSpace(raw)
		if l == "" || strings.HasPrefix(l, "//") || strings.HasPrefix(l, "#") {
			continue
		}
		if headerRe.MatchString(l) {
			group = normalizeGroup(l)
			continue
		}
		if strings.EqualFold(l, "about") {
			about = true
			continue
		}
		if about && strings.HasPrefix(strings.ToLower(l), "name ") {
			about = false
			continue
		}
		row := listRow{Line: i + 1, Text: l, Qty: 1, Group: group}
		if q, ok := strings.CutPrefix(l, "?"); ok {
			row.Query = strings.TrimSpace(q)
			if row.Query != "" {
				rows = append(rows, row)
			}
			continue
		}
		body := l
		if format != formatNames {
			if m := quantityRe.FindStringSubmatch(l); m != nil {
				n, _ := strconv.Atoi(m[1])
				row.Qty = min(max(n, 1), maxQuantity)
				body = m[2]
			}
		}
		if format == formatArena {
			if m := arenaRe.FindStringSubmatch(body); m != nil {
				body, row.Set, row.Number = m[1], strings.ToLower(m[2]), m[3]
			}
		}
		row.Name = strings.TrimSpace(body)
		rows = append(rows, row)
	}
	return rows
}

// normalizeGroup turns a header line into the label the table filters by, so
// Sideboard, SIDEBOARD: and side all read as one group
func normalizeGroup(header string) string {
	h := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(header), ":")))
	switch h {
	case "main", "mainboard", "maindeck", "deck":
		return "Deck"
	case "side", "sideboard":
		return "Sideboard"
	case "considering", "maybeboard":
		return "Maybeboard"
	}
	return strings.ToUpper(h[:1]) + h[1:]
}

// csvAliases maps header spellings a spreadsheet might use onto edit field keys
var csvAliases = map[string]string{
	"cost":             "manaCost",
	"mana":             "manaCost",
	"mana cost":        "manaCost",
	"type":             "typeLine",
	"type line":        "typeLine",
	"text":             "oracle",
	"rules":            "oracle",
	"oracle text":      "oracle",
	"flavor text":      "flavor",
	"set":              "setCode",
	"set code":         "setCode",
	"number":           "collector",
	"cn":               "collector",
	"collector number": "collector",
	"released at":      "released",
	"lang":             "language",
	"qty":              "qty",
	"count":            "qty",
	"quantity":         "qty",
	"group":            "group",
	"section":          "group",
	"custom":           "custom",
}

// editFieldKeys is the set of JSON keys editFields carries, read from its tags
// so the CSV columns can never drift from what the editor and a render accept
var editFieldKeys = func() map[string]bool {
	raw, _ := json.Marshal(editFields{})
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	keys := make(map[string]bool, len(m))
	for k := range m {
		keys[k] = true
	}
	return keys
}()

// csvColumns maps each header cell to its key: an edit field, qty, group or
// custom, or "" for a column that is ignored. It returns nil when no column is
// recognized, which is how sniffing tells a header from a line with a comma
func csvColumns(header []string) []string {
	cols := make([]string, len(header))
	known := false
	lowerKeys := make(map[string]string, len(editFieldKeys))
	for k := range editFieldKeys {
		lowerKeys[strings.ToLower(k)] = k
	}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		key := csvAliases[h]
		if key == "" {
			key = lowerKeys[strings.ReplaceAll(h, " ", "")]
		}
		if key != "" {
			cols[i] = key
			known = true
		}
	}
	if !known {
		return nil
	}
	return cols
}

// parseCSV reads a CSV list whose header row names its columns. The name column
// becomes the row's lookup name, and every other edit field column is carried
// in Fields to overlay onto whatever the lookup returns
func parseCSV(text string) ([]listRow, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	cols := csvColumns(records[0])
	if cols == nil {
		return nil, fmt.Errorf("the CSV header names no known column, such as name")
	}
	var rows []listRow
	for i, rec := range records[1:] {
		row := listRow{Line: i + 2, Text: strings.Join(rec, ","), Qty: 1}
		empty := true
		for j, v := range rec {
			v = strings.TrimSpace(v)
			if j >= len(cols) || cols[j] == "" || v == "" {
				continue
			}
			empty = false
			switch cols[j] {
			case "qty":
				if n, err := strconv.Atoi(v); err == nil {
					row.Qty = min(max(n, 1), maxQuantity)
				}
			case "group":
				row.Group = v
			case "custom":
				row.Custom = truthy(v)
			case "name":
				row.Name = v
				fallthrough
			default:
				if row.Fields == nil {
					row.Fields = make(map[string]string)
				}
				row.Fields[cols[j]] = v
			}
		}
		if empty {
			continue
		}
		// A lone name is a plain lookup, so it carries no overrides
		if len(row.Fields) == 1 && row.Name != "" {
			row.Fields = nil
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "y", "yes", "true", "x":
		return true
	}
	return false
}
