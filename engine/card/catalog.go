package card

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/odevine/mimic/engine/card/svgpath"
)

// maxCatalogEntries bounds how many files a catalog zip may list, far above the
// few thousand a real one holds
const maxCatalogEntries = 20000

// catalog is the mtg-vectors set symbol collection, read from its release zip.
// Each set icon has a folder of files named for the rarity they are colored
// for, such as set/M21/R.svg, and a manifest.json maps Scryfall's sets and icon
// codes onto those folders
type catalog struct {
	files map[string]*zip.File // keyed by path, such as "set/M21/R.svg"
	// symbols lists each folder's rarity letters, routes sends a set code to a
	// folder other than its own icon's, and aliases renames an icon code
	symbols map[string][]string
	routes  map[string]string
	aliases map[string]string
}

// openCatalog reads a catalog zip held in memory
func openCatalog(data []byte) (*catalog, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("reading the symbol catalog: %w", err)
	}
	if len(zr.File) > maxCatalogEntries {
		return nil, fmt.Errorf("symbol catalog lists %d files", len(zr.File))
	}
	c := &catalog{files: make(map[string]*zip.File)}
	var manifest *zip.File
	for _, f := range zr.File {
		switch parts := strings.Split(f.Name, "/"); {
		case f.Name == "manifest.json":
			manifest = f
		case len(parts) == 3 && parts[0] == "set" && !strings.HasPrefix(parts[1], ".") && strings.HasSuffix(parts[2], ".svg"):
			c.files[f.Name] = f
		}
	}
	if manifest == nil {
		return nil, fmt.Errorf("symbol catalog has no manifest.json")
	}
	raw, err := readEntry(manifest, 8<<20)
	if err != nil {
		return nil, fmt.Errorf("reading the symbol catalog's manifest: %w", err)
	}
	var m struct {
		Set struct {
			Aliases map[string]string   `json:"aliases"`
			Routes  map[string]string   `json:"routes"`
			Symbols map[string][]string `json:"symbols"`
		} `json:"set"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decoding the symbol catalog's manifest: %w", err)
	}
	if len(m.Set.Symbols) == 0 {
		return nil, fmt.Errorf("symbol catalog lists no symbols")
	}
	c.symbols, c.routes, c.aliases = m.Set.Symbols, m.Set.Routes, m.Set.Aliases
	return c, nil
}

// readEntry reads one zip entry, refusing one whose declared or actual size is
// over max
func readEntry(f *zip.File, max int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(max) {
		return nil, fmt.Errorf("%s is %d bytes, over the %d byte limit", f.Name, f.UncompressedSize64, max)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return readBounded(rc, max)
}

// folder is the catalog folder a set's symbol lives in. A route on the set code
// wins, then an alias of the icon Scryfall names for the set, and otherwise the
// icon itself. It reports false for a set whose icon the catalog does not have
func (c *catalog) folder(setCode, iconCode string) (string, bool) {
	code := strings.ToUpper(iconCode)
	if to, ok := c.routes[strings.ToUpper(setCode)]; ok {
		code = strings.ToUpper(to)
	} else if to, ok := c.aliases[code]; ok {
		code = strings.ToUpper(to)
	}
	_, ok := c.symbols[code]
	return code, ok
}

// rarityLetters lists the file letters to try for a card's rarity, best first.
// A bonus card prints in the mythic color, and a special one is a timeshifted
// card or an unusual printing, so it takes the special or timeshifted color
// where the set has one. Common is the last resort for any of them
func rarityLetters(rarity string) []string {
	switch strings.ToLower(strings.TrimSpace(rarity)) {
	case "uncommon":
		return []string{"U", "C"}
	case "rare":
		return []string{"R", "C"}
	case "mythic":
		return []string{"M", "R", "C"}
	case "bonus":
		return []string{"B", "M", "R", "C"}
	case "special":
		return []string{"S", "T", "M", "R", "C"}
	}
	return []string{"C"}
}

// icon reads the symbol for a rarity from a folder, returning the letter whose
// file it used. It reports false when the folder has none of the letters
func (c *catalog) icon(folder, rarity string) (*svgpath.Icon, string, bool, error) {
	for _, letter := range rarityLetters(rarity) {
		f, ok := c.files["set/"+folder+"/"+letter+".svg"]
		if !ok {
			continue
		}
		data, err := readEntry(f, svgpath.MaxBytes)
		if err != nil {
			return nil, "", false, err
		}
		icon, err := svgpath.Parse(data)
		if err != nil {
			return nil, "", false, err
		}
		return icon, letter, true, nil
	}
	return nil, "", false, nil
}
