package prefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyFixture puts a settings file written by an earlier release where Load looks
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "prefs.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A settings file from before the three templates became one keeps working, with
// its choices moved to the template that replaced them and nothing else changed
func TestLoadMovesTheEarlierTemplatesToTheOneThatReplacedThem(t *testing.T) {
	path := copyFixture(t, "prefs-0.9.json")
	s := Load(path)

	if name, version := s.Template(); name != DefaultTemplate || version != "local" {
		t.Errorf("template = %s %s, want %s local", name, version, DefaultTemplate)
	}
	faces := s.FaceTemplates()
	want := map[string]TemplateChoice{
		"split/standard":           {Name: DefaultTemplate},
		"transform_back/standard":  {Name: DefaultTemplate},
		"transform_front/standard": {Name: DefaultTemplate, Version: "local"},
	}
	for key, w := range want {
		if faces[key] != w {
			t.Errorf("%s = %+v, want %+v", key, faces[key], w)
		}
	}
	if got := s.RecentSearches(); len(got) != 2 || got[0] != "lightning bolt" {
		t.Errorf("recent searches = %v", got)
	}
	if preview, _ := s.Resolution(); preview != 300 {
		t.Errorf("preview dpi = %d, want 300", preview)
	}
	if got := s.Settings(); got.Theme != "dark" || got.LayerCache != "large" {
		t.Errorf("settings = %+v", got)
	}

	// The new form is written back, and loads the same
	again := Load(path)
	if name, version := again.Template(); name != DefaultTemplate || version != "local" {
		t.Errorf("reloaded template = %s %s", name, version)
	}
	raw, _ := os.ReadFile(path)
	for _, old := range []string{`"normal"`, `"split"`, `"transform"`} {
		if strings.Contains(string(raw), old) {
			t.Errorf("the file still names an earlier template, %s:\n%s", old, raw)
		}
	}
}

func TestLoadLeavesAMigratedFileAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	if err := os.WriteFile(path, []byte(`{"templateName":"mimic.m15","templateVersion":"0.2.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	s := Load(path)
	if name, version := s.Template(); name != "mimic.m15" || version != "0.2.0" {
		t.Errorf("template = %s %s", name, version)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("a file with nothing to migrate was written")
	}
}

func TestFeaturesAreKeptPerTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	s := Load(path)
	s.SetFeature("mimic.m15", "ptStyle", "dark")
	s.SetFeature("mimic.m15", "glow", 0.5)
	s.SetFeature("other.frame", "ptStyle", "light")

	got := Load(path).Features("mimic.m15")
	if got["ptStyle"] != "dark" || got["glow"] != 0.5 || len(got) != 2 {
		t.Errorf("features = %v", got)
	}
	s.SetFeature("mimic.m15", "ptStyle", nil)
	if got := s.Features("mimic.m15"); got["ptStyle"] != nil || got["glow"] != 0.5 {
		t.Errorf("after forgetting one = %v", got)
	}
	s.SetFeature("mimic.m15", "glow", nil)
	if len(s.Features("mimic.m15")) != 0 || len(Load(path).Features("other.frame")) != 1 {
		t.Error("forgetting every feature of a template disturbed another")
	}
}
