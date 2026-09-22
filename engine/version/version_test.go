package version

import "testing"

func TestSatisfies(t *testing.T) {
	cases := []struct {
		name      string
		running   string
		minEngine string
		want      bool
	}{
		{"dev renders anything", "dev", "9.9.9", true},
		{"empty running renders anything", "", "9.9.9", true},
		{"no constraint", "0.3.0", "", true},
		{"newer running satisfies", "0.4.0", "0.3.0", true},
		{"equal satisfies", "0.3.0", "0.3.0", true},
		{"older running rejected", "0.3.0", "0.4.0", false},
		{"v-prefixed minEngine", "0.3.0", "v0.3.0", true},
		{"invalid minEngine rejected", "0.3.0", "not-a-version", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := Version
			Version = c.running
			defer func() { Version = old }()
			if got := Satisfies(c.minEngine); got != c.want {
				t.Errorf("Satisfies(running=%q, min=%q) = %v, want %v", c.running, c.minEngine, got, c.want)
			}
		})
	}
}
