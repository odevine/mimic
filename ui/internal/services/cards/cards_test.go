package cards

import (
	"strings"
	"testing"
)

func TestPrintingsQuery(t *testing.T) {
	q := printingsQuery("Lightning Bolt")
	if !strings.HasPrefix(q, `!"Lightning Bolt"`) || !strings.Contains(q, "unique:prints") {
		t.Errorf("printingsQuery = %q, want an exact-name unique:prints search", q)
	}
}
