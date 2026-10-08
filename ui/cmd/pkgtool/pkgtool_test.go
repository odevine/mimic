package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestStampWritesTheVersionEverywhereItLives(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "darwin/Info.plist"), "<key>CFBundleVersion</key>\n  <string>0.0.0</string>\n<key>CFBundleShortVersionString</key>\n  <string>0.0.0</string>\n<key>CFBundleName</key>\n  <string>Mimic</string>")
	write(t, filepath.Join(dir, "windows/info.json"), `{"fixed": {"file_version": "0.0.0"}, "info": {"0000": {"ProductVersion": "0.0.0", "ProductName": "Mimic"}}}`)
	write(t, filepath.Join(dir, "config.yml"), "info:\n  productName: \"Mimic\"\n  version: \"0.0.0\"\n")

	if err := stamp(dir, "v1.2.3-rc.1"); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string][]string{
		"darwin/Info.plist": {"<string>1.2.3</string>\n<key>CFBundleShortVersionString</key>\n  <string>1.2.3</string>", "<string>Mimic</string>"},
		"windows/info.json": {`"file_version": "1.2.3"`, `"ProductVersion": "1.2.3"`, `"ProductName": "Mimic"`},
		"config.yml":        {`version: "1.2.3"`, `productName: "Mimic"`},
	} {
		raw, _ := os.ReadFile(filepath.Join(dir, file))
		for _, w := range want {
			if !strings.Contains(string(raw), w) {
				t.Errorf("%s lacks %q:\n%s", file, w, raw)
			}
		}
	}
	if err := stamp(dir, "next"); err == nil {
		t.Error("a version that is not a number was stamped")
	}
	if err := stamp(t.TempDir(), "1.0.0"); err == nil {
		t.Error("an empty assets folder was stamped")
	}
}

func TestZipHasOneTopLevelEntryAndKeepsPermissions(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "bin/Mimic.app/Contents/MacOS/mimic"), "binary")
	write(t, filepath.Join(dir, "bin/Mimic.app/Contents/Info.plist"), "plist")
	out := filepath.Join(dir, "Mimic.zip")
	if err := zipOne(out, filepath.Join(dir, "bin/Mimic.app")); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	tops := map[string]bool{}
	for _, f := range zr.File {
		tops[strings.SplitN(f.Name, "/", 2)[0]] = true
		if f.Name == "Mimic.app/Contents/MacOS/mimic" && f.Mode()&0o100 == 0 {
			t.Errorf("the binary lost its executable bit: %v", f.Mode())
		}
	}
	if len(tops) != 1 || !tops["Mimic.app"] {
		t.Errorf("top level entries = %v, want only Mimic.app", tops)
	}

	// A single file, as the Windows executable is
	write(t, filepath.Join(dir, "bin/Mimic.exe"), "exe")
	if err := zipOne(filepath.Join(dir, "exe.zip"), filepath.Join(dir, "bin/Mimic.exe")); err != nil {
		t.Fatal(err)
	}
	zr2, _ := zip.OpenReader(filepath.Join(dir, "exe.zip"))
	defer zr2.Close()
	if len(zr2.File) != 1 || zr2.File[0].Name != "Mimic.exe" {
		t.Errorf("exe zip = %v", zr2.File)
	}
}

func TestSumsAreSortedAndNamedByFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "b.zip"), "bee")
	write(t, filepath.Join(dir, "a.dmg"), "ay")
	out := filepath.Join(dir, "SHA256SUMS")
	if err := writeSums(out, []string{filepath.Join(dir, "b.zip"), filepath.Join(dir, "a.dmg")}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "  a.dmg") && !strings.HasSuffix(lines[1], "  a.dmg") || len(strings.Fields(lines[0])[0]) != 64 {
		t.Errorf("sums = %q", raw)
	}
	if lines[0] > lines[1] {
		t.Errorf("sums are not sorted: %q", raw)
	}
}

func TestSignatureVerifiesAgainstTheDigest(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	path := filepath.Join(dir, "Mimic.zip")
	write(t, path, "release bytes")
	t.Setenv(keyEnv, base64.StdEncoding.EncodeToString(priv))
	if err := signCmd([]string{path}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path + ".sig")
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(sig) != 64 {
		t.Fatalf("signature file = %q, %v", raw, err)
	}
	d, _ := digest(path)
	if !ed25519.Verify(pub, d, sig) {
		t.Error("the signature does not verify against the file's digest")
	}
	t.Setenv(keyEnv, "")
	if err := signCmd([]string{path}); err == nil {
		t.Error("signing worked with no key")
	}
}

func TestRenameMovesAndCopies(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.exe"), "exe")
	if err := renameCmd([]string{"--copy", filepath.Join(dir, "a.exe"), filepath.Join(dir, "stage/Mimic.exe")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.exe")); err != nil {
		t.Error("a copy removed the original")
	}
	if err := renameCmd([]string{filepath.Join(dir, "a.exe"), filepath.Join(dir, "out/b.exe")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.exe")); err == nil {
		t.Error("a move left the original")
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "out/b.exe")); string(raw) != "exe" {
		t.Errorf("moved file = %q", raw)
	}
}

func TestLinuxNamesGivesTheReleaseNames(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, "stage")
	for _, f := range []string{"mimic-x86_64.AppImage", "mimic_1.0.0_amd64.deb", "mimic-1.0.0-1.x86_64.rpm"} {
		write(t, filepath.Join(stage, f), f)
	}
	if err := linuxNames(stage, dir, "1.0.0", "amd64"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Mimic-1.0.0-linux-amd64.AppImage", "mimic_1.0.0_amd64.deb", "mimic-1.0.0.x86_64.rpm"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s", want)
		}
	}
	if err := linuxNames(stage, dir, "1.0.0", "amd64"); err == nil {
		t.Error("a second run with nothing left succeeded")
	}
}
