package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MergePatch applies a JSON merge patch to a JSON document and returns the
// result. An object in the patch merges into the one in the document, a null
// removes the key, and any other value replaces what is there. An object is the
// only thing that merges, so a patch replaces a list whole
func MergePatch(doc, patch json.RawMessage) (json.RawMessage, error) {
	var d, p any
	if err := json.Unmarshal(doc, &d); err != nil {
		return nil, fmt.Errorf("document: %w", err)
	}
	if err := json.Unmarshal(patch, &p); err != nil {
		return nil, fmt.Errorf("patch: %w", err)
	}
	return json.Marshal(merge(d, p))
}

func merge(doc, patch any) any {
	pm, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	dm, ok := doc.(map[string]any)
	if !ok {
		dm = map[string]any{}
	}
	out := make(map[string]any, len(dm))
	for k, v := range dm {
		out[k] = v
	}
	for k, v := range pm {
		if v == nil {
			delete(out, k)
			continue
		}
		out[k] = merge(out[k], v)
	}
	return out
}

// decodeStrict reads JSON into v and fails on a field v does not have, so a
// patch that names something a text box lacks is caught where it is written
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
