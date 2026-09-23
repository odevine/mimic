package main

import (
	"reflect"
	"testing"
)

func TestSniffFormat(t *testing.T) {
	cases := map[string]string{
		"":                                          formatNames,
		"Lightning Bolt\nCounterspell":              formatNames,
		"4 Lightning Bolt\nCounterspell":            formatQuantity,
		"4x Lightning Bolt":                         formatQuantity,
		"4 Lightning Bolt (2X2) 117\n1 Opt":         formatArena,
		"Deck\n1 Sol Ring (C21) 263":                formatArena,
		"name,power\nGoblin Guide,2":                formatCSV,
		"Name, Mana Cost\nX,{R}":                    formatCSV,
		"Fire // Ice, the best card":                formatNames,
		"// comment\n?t:goblin c:r\nLightning Bolt": formatNames,
	}
	for in, want := range cases {
		if got := sniffFormat(in); got != want {
			t.Errorf("sniffFormat(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseLines(t *testing.T) {
	text := "About\nName My Deck\n\nDeck\n4 Lightning Bolt (2X2) 117\n1x Cryptic Command\n// a comment\nOpt (XLN) 65 *F*\n\nSideboard:\n2 Rest in Peace\n?t:goblin c:r\n"
	rows, format, err := parseList(text, "")
	if err != nil {
		t.Fatal(err)
	}
	if format != formatArena {
		t.Errorf("format = %q, want arena", format)
	}
	want := []listRow{
		{Line: 5, Text: "4 Lightning Bolt (2X2) 117", Qty: 4, Name: "Lightning Bolt", Set: "2x2", Number: "117", Group: "Deck"},
		{Line: 6, Text: "1x Cryptic Command", Qty: 1, Name: "Cryptic Command", Group: "Deck"},
		{Line: 8, Text: "Opt (XLN) 65 *F*", Qty: 1, Name: "Opt", Set: "xln", Number: "65", Group: "Deck"},
		{Line: 11, Text: "2 Rest in Peace", Qty: 2, Name: "Rest in Peace", Group: "Sideboard"},
		{Line: 12, Text: "?t:goblin c:r", Qty: 1, Query: "t:goblin c:r", Group: "Sideboard"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows =\n%+v\nwant\n%+v", rows, want)
	}
}

func TestParseFormatOverride(t *testing.T) {
	rows, _, err := parseList("1996 World Champion", formatNames)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Name != "1996 World Champion" || rows[0].Qty != 1 {
		t.Errorf("names format read %+v", rows[0])
	}
	rows, _, _ = parseList("4 Lightning Bolt (2X2) 117", formatQuantity)
	if rows[0].Name != "Lightning Bolt (2X2) 117" || rows[0].Set != "" {
		t.Errorf("quantity format should keep the printing in the name, got %+v", rows[0])
	}
	if _, _, err := parseList("x", "yaml"); err == nil {
		t.Error("unknown format should fail")
	}
}

func TestParseQuantityBounds(t *testing.T) {
	rows, _, _ := parseList("0 Island\n5000 Forest", "")
	if rows[0].Qty != 1 || rows[1].Qty != maxQuantity {
		t.Errorf("quantities = %d, %d", rows[0].Qty, rows[1].Qty)
	}
}

func TestParseCSV(t *testing.T) {
	text := "Name,Qty,Type,Power,Toughness,Custom,Ignored\n" +
		"Goblin Guide,4,,,,,zzz\n" +
		"Big Dragon,1,Creature — Dragon,9,9,yes,\n" +
		"Grizzly Bears,2,,3,3,,\n" +
		",,,,,,\n"
	rows, format, err := parseList(text, "")
	if err != nil {
		t.Fatal(err)
	}
	if format != formatCSV {
		t.Fatalf("format = %q", format)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0].Name != "Goblin Guide" || rows[0].Qty != 4 || rows[0].Fields != nil {
		t.Errorf("plain row = %+v", rows[0])
	}
	if !rows[1].Custom || rows[1].Fields["typeLine"] != "Creature — Dragon" || rows[1].Fields["power"] != "9" {
		t.Errorf("custom row = %+v", rows[1])
	}
	if rows[2].Fields["power"] != "3" || rows[2].Fields["name"] != "Grizzly Bears" {
		t.Errorf("override row = %+v", rows[2])
	}
}

func TestParseCSVNeedsKnownColumn(t *testing.T) {
	if _, _, err := parseList("foo,bar\n1,2", formatCSV); err == nil {
		t.Error("a header with no known column should fail")
	}
}

func TestParseRowLimit(t *testing.T) {
	text := ""
	for range maxListRows + 1 {
		text += "Island\n"
	}
	if _, _, err := parseList(text, ""); err == nil {
		t.Error("a list past the row limit should fail")
	}
}
