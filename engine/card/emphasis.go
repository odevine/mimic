package card

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed emphasis_words.json
var emphasisWordsJSON []byte

// EmphasisWords are the lowercased ability words and flavor words that head an
// ability and print in italics before an em-dash, such as Constellation or a
// flavor word like "Learn Their Secrets". Keyword abilities and actions are not
// included, so keywords like Suspend or Cycling stay roman. This is card data,
// not a template concern, so every template shares one set.
//
// It is the union of Scryfall's ability-words and flavor-words catalogs. To
// refresh, overwrite emphasis_words.json with the lowercased union of:
//
//	https://api.scryfall.com/catalog/ability-words
//	https://api.scryfall.com/catalog/flavor-words
var EmphasisWords = loadEmphasisWords()

func loadEmphasisWords() map[string]bool {
	var words []string
	if err := json.Unmarshal(emphasisWordsJSON, &words); err != nil {
		panic("card: invalid emphasis_words.json: " + err.Error())
	}
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[strings.ToLower(w)] = true
	}
	return set
}
