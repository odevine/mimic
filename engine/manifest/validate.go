package manifest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/frame"
)

var (
	idRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.[a-z0-9][a-z0-9-]*$`)
	partNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	boxNameRe  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	featureRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	valueRe    = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	keyRe      = regexp.MustCompile(`^[a-z0-9_]+$`)
	colorRe    = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	roleRe     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	fontIDRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	fontFileRe = regexp.MustCompile(`^font/[A-Za-z0-9][A-Za-z0-9 ._-]*\.(?i:ttf|otf)$`)
	sha256Re   = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// colorKeys are the material keys a part that reads a frame color can hold
var colorKeys = func() map[string]bool {
	keys := map[string]bool{"gold": true, "artifact": true, "colorless": true, "land": true, "vehicle": true, "any": true}
	const letters = "wubrg"
	for i := range letters {
		keys[letters[i:i+1]] = true
		for j := i + 1; j < len(letters); j++ {
			keys[letters[i:i+1]+letters[j:j+1]] = true
		}
	}
	return keys
}()

// blendSlots are the slots whose material can be a run of colors
var blendSlots = map[frame.Slot]bool{
	frame.SlotBackground: true, frame.SlotPinlines: true, frame.SlotCrown: true,
	frame.SlotFuse: true, frame.SlotFusePinlines: true,
}

// Validate checks a manifest and fills in what it leaves to a default: an entry
// with no ID takes its part's name, and each text box override gets the box it
// makes. It reports every problem it finds, not the first
func Validate(m *Manifest) Diagnostics {
	v := &validator{m: m}
	v.manifest()
	return v.c.ds
}

type validator struct {
	m *Manifest
	c collector
}

func (v *validator) manifest() {
	m := v.m
	if !idRe.MatchString(m.ID) {
		v.c.errorf("id", "%q is not an id like %q, which is lowercase letters, digits, and hyphens in an owner and a name joined by a dot", m.ID, "mimic.m15")
	}
	if strings.TrimSpace(m.Name) == "" {
		v.c.errorf("name", "the template has no name")
	}
	if m.Description == "" {
		v.c.warnf("description", "the template has no description")
	}
	for i, cr := range m.Credits {
		if strings.TrimSpace(cr.Name) == "" {
			v.c.errorf(fmt.Sprintf("credits[%d].name", i), "a credit has no name")
		}
	}
	v.caps()
	for _, name := range sortedKeys(m.Features) {
		v.feature("features."+name, name, m.Features[name])
	}
	for _, name := range sortedKeys(m.Parts) {
		v.part("parts."+name, name, m.Parts[name])
	}
	v.fonts()
	if len(m.Layouts) == 0 {
		v.c.errorf("layouts", "the template has no layouts")
	}
	served := map[frame.Shape]string{}
	used := map[string]bool{}
	for _, name := range sortedKeys(m.Layouts) {
		l := m.Layouts[name]
		v.layout("layouts."+name, name, &l, served, used)
		m.Layouts[name] = l
	}
	for _, name := range sortedKeys(m.Parts) {
		if !used[name] {
			v.c.warnf("parts."+name, "no layout uses the part")
		}
	}
}

// fonts checks the fonts a template bundles and the roles it defines
func (v *validator) fonts() {
	m := v.m
	if (len(m.Fonts) > 0 || len(m.Roles) > 0) && !contractAtLeast(m.Contract, 1) {
		v.c.errorf("contract", "fonts and roles need contract 1.1 or newer, and this is %s", m.Contract)
	}
	used := map[string]bool{}
	for _, name := range sortedKeys(m.Roles) {
		r := m.Roles[name]
		path := "roles." + name
		if !roleRe.MatchString(name) {
			v.c.errorf(path, "%q is not a role name, which is lowercase letters, digits, and hyphens starting with a letter", name)
		}
		if len(r.Label) > 80 {
			v.c.errorf(path+".label", "is over 80 characters")
		}
		switch {
		case r.Font != "" && r.Builtin != "":
			v.c.errorf(path, "names both a font and a built-in font, and has to name one")
		case r.Font != "":
			used[r.Font] = true
			if _, ok := m.Fonts[r.Font]; !ok {
				v.c.errorf(path+".font", "the template bundles no font %q", r.Font)
			}
		case r.Builtin != "":
			if !fonts.IsBuiltin(r.Builtin) {
				v.c.errorf(path+".builtin", "%q is not a built-in font", r.Builtin)
			}
		default:
			v.c.errorf(path, "names no font, so give it a font or a built-in font")
		}
	}
	files := map[string]string{}
	for _, id := range sortedKeys(m.Fonts) {
		f := m.Fonts[id]
		path := "fonts." + id
		if !fontIDRe.MatchString(id) {
			v.c.errorf(path, "%q is not a font id, which is lowercase letters, digits, hyphens, and underscores", id)
		}
		switch {
		case len(f.File) > MaxPathBytes:
			v.c.errorf(path+".file", "is over %d bytes", MaxPathBytes)
		case !fontFileRe.MatchString(f.File):
			v.c.errorf(path+".file", "%q is not a .ttf or .otf file directly in font/", f.File)
		}
		if other, dup := files[f.File]; dup && f.File != "" {
			v.c.warnf(path+".file", "is also the file of %s", other)
		}
		files[f.File] = id
		if f.SHA256 != "" && !sha256Re.MatchString(f.SHA256) {
			v.c.errorf(path+".sha256", "is not 64 lowercase hex digits")
		}
		if !used[id] {
			v.c.warnf(path, "no role uses the font")
		}
	}
}

// contractAtLeast reports whether a contract version has at least the given minor
// of the current major
func contractAtLeast(contract string, minor int) bool {
	match := contractRe.FindStringSubmatch(contract)
	if match == nil {
		return false
	}
	got, _ := strconv.Atoi(match[2])
	return got >= minor
}

func (v *validator) caps() {
	m := v.m
	check := func(path string, n, limit int) {
		if n > limit {
			v.c.errorf(path, "there are %d, over the limit of %d", n, limit)
		}
	}
	check("parts", len(m.Parts), MaxParts)
	check("layouts", len(m.Layouts), MaxLayouts)
	check("features", len(m.Features), MaxFeatures)
	check("fonts", len(m.Fonts), MaxFonts)
	check("roles", len(m.Roles), MaxRoles)
}

func (v *validator) feature(path, name string, f Feature) {
	if !featureRe.MatchString(name) {
		v.c.errorf(path, "%q is not a feature name, which is letters, digits, and underscores and does not start with a digit", name)
	}
	var decodeErr error
	decode := func(into any) {
		if len(f.Default) > 0 {
			decodeErr = json.Unmarshal(f.Default, into)
		}
	}
	switch f.Type {
	case "bool":
		if len(f.Values) > 0 || f.Min != 0 || f.Max != 0 || f.Step != 0 {
			v.c.errorf(path, "a bool feature has no values or range")
		}
		var d bool
		decode(&d)
	case "choice":
		if len(f.Values) == 0 {
			v.c.errorf(path+".values", "a choice needs at least one value")
		}
		seen := map[string]bool{}
		for i, val := range f.Values {
			switch {
			case !valueRe.MatchString(val):
				v.c.errorf(fmt.Sprintf("%s.values[%d]", path, i), "%q is not a value, which is letters, digits, dots, underscores, and hyphens", val)
			case seen[val]:
				v.c.errorf(fmt.Sprintf("%s.values[%d]", path, i), "%q is listed twice", val)
			}
			seen[val] = true
		}
		var d string
		decode(&d)
		if decodeErr == nil && len(f.Default) > 0 && !slices.Contains(f.Values, d) {
			v.c.errorf(path+".default", "%q is not one of the values", d)
		}
	case "range":
		if f.Min >= f.Max {
			v.c.errorf(path, "the range runs from %g to %g, which holds nothing", f.Min, f.Max)
		}
		if f.Step <= 0 || f.Step > f.Max-f.Min {
			v.c.errorf(path+".step", "the step %g does not fit a range of %g", f.Step, f.Max-f.Min)
		}
		if len(f.Values) > 0 {
			v.c.errorf(path+".values", "a range has no values")
		}
		var d float64
		decode(&d)
		if decodeErr == nil && len(f.Default) > 0 && (d < f.Min || d > f.Max) {
			v.c.errorf(path+".default", "%g is outside the range %g to %g", d, f.Min, f.Max)
		}
	default:
		v.c.errorf(path+".type", "%q is not a feature type, which is bool, choice, or range", f.Type)
	}
	if decodeErr != nil {
		v.c.errorf(path+".default", "the default does not fit a %s feature: %v", f.Type, decodeErr)
	}
}

func (v *validator) part(path, name string, p Part) {
	if !partNameRe.MatchString(name) {
		v.c.errorf(path, "%q is not a part name, which is lowercase letters, digits, underscores, and hyphens", name)
	}
	slot, err := frame.ParseSlot(p.Slot)
	if err != nil {
		v.c.errorf(path+".slot", "%v", err)
	}
	if len(p.Variants) == 0 {
		v.c.errorf(path+".variants", "the part has no images")
	}
	if len(p.Variants) > MaxVariants {
		v.c.errorf(path+".variants", "there are %d images, over the limit of %d", len(p.Variants), MaxVariants)
	}
	for _, key := range sortedKeys(p.Variants) {
		vp := path + ".variants." + key
		if msg, sev := variantKeyProblem(slot, key); msg != "" {
			if sev == Warning {
				v.c.warnf(vp, "%s", msg)
			} else {
				v.c.errorf(vp, "%s", msg)
			}
		}
		if msg := pathProblem(p.Variants[key].Path); msg != "" {
			v.c.errorf(vp+".path", "%s", msg)
		}
	}
	if p.Origin.X < 0 || p.Origin.Y < 0 || p.Origin.X > MaxDimension || p.Origin.Y > MaxDimension {
		v.c.errorf(path+".origin", "(%d, %d) is outside the canvas", p.Origin.X, p.Origin.Y)
	}
	if p.Blend != nil {
		v.blend(path+".blend", slot, p.Blend)
	}
}

// variantKeyProblem says why a key cannot name an image of a part with the slot,
// or "" when it can. A transform icon the engine does not know is a warning and
// not an error, since a newer template can carry art for an icon an older engine
// does not select yet
func variantKeyProblem(slot frame.Slot, key string) (string, Severity) {
	if !keyRe.MatchString(key) {
		return fmt.Sprintf("%q is not an image key, which is lowercase letters, digits, and underscores", key), Error
	}
	if key == "any" {
		return "", Error
	}
	switch slot {
	case frame.SlotNone:
		return fmt.Sprintf("a part with no slot has only an \"any\" image, not %q", key), Error
	case frame.SlotIndicator:
		if !isColorSet(key) {
			return fmt.Sprintf("%q is not a set of colors in WUBRG order, which names an indicator's image", key), Error
		}
	case frame.SlotTransformIcon:
		if !slices.Contains(frame.TransformIconNames(), key) {
			return fmt.Sprintf("%q is not an icon this engine selects, which are %s, so it never draws", key, strings.Join(frame.TransformIconNames(), ", ")), Warning
		}
	default:
		if !colorKeys[key] {
			return fmt.Sprintf("%q is not a material, which is a color, a pair in WUBRG order, gold, artifact, colorless, land, or vehicle", key), Error
		}
	}
	return "", Error
}

// isColorSet reports whether s is one or more of the letters wubrg, each once and
// in that order
func isColorSet(s string) bool {
	const order = "wubrg"
	last := -1
	for _, r := range s {
		i := strings.IndexRune(order, r)
		if i <= last {
			return false
		}
		last = i
	}
	return s != ""
}

// pathProblem says why a path cannot name an image in a bundle, or "" when it can
func pathProblem(p string) string {
	switch {
	case p == "":
		return "the image has no path"
	case len(p) > MaxPathBytes:
		return fmt.Sprintf("the path is %d bytes, over the limit of %d", len(p), MaxPathBytes)
	case strings.ContainsAny(p, "\\:\x00"):
		return fmt.Sprintf("%q has a backslash, a colon, or a null, so it is not a slash separated path inside the bundle", p)
	case strings.HasPrefix(p, "/"):
		return fmt.Sprintf("%q is absolute, so it is not inside the bundle", p)
	case !strings.HasSuffix(strings.ToLower(p), ".png"):
		return fmt.Sprintf("%q is not a PNG", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Sprintf("%q has an empty, \".\", or \"..\" part, so it may leave the bundle", p)
		}
	}
	return ""
}

func (v *validator) blend(path string, slot frame.Slot, b *Blend) {
	if !blendSlots[slot] {
		v.c.warnf(path, "a part on the %q slot never reads a run of colors, so this does nothing", slot)
	}
	switch b.Shape {
	case "", "smooth", "linear":
	default:
		v.c.errorf(path+".shape", "%q is not a shape, which is smooth or linear", b.Shape)
	}
	if b.End < 0 || b.End > 1 {
		v.c.errorf(path+".end", "%g is outside 0 to 1", b.End)
	}
	for _, count := range sortedKeys(b.Seams) {
		n, err := strconv.Atoi(count)
		if err != nil || n < 2 || n > 4 {
			v.c.errorf(path+".seams."+count, "%q is not a number of colors, which is 2, 3, or 4", count)
			continue
		}
		seams := b.Seams[count]
		if len(seams) != n-1 {
			v.c.errorf(path+".seams."+count, "a run of %d colors has %d seams, and there are %d", n, n-1, len(seams))
		}
		for i, s := range seams {
			sp := fmt.Sprintf("%s.seams.%s[%d]", path, count, i)
			if s.Width <= 0 || s.Width > 2*MaxDimension {
				v.c.errorf(sp+".width", "%g is not a width from above 0 to %d", s.Width, 2*MaxDimension)
			}
			if s.Center < -MaxDimension || s.Center > MaxDimension {
				v.c.errorf(sp+".center", "%g is off the canvas", s.Center)
			}
			// Each seam starts after the one before it ends, or a color's weight would go negative
			if i > 0 && s.Center-s.Width/2 < seams[i-1].Center+seams[i-1].Width/2 {
				v.c.errorf(sp, "the seam starts before the one before it ends")
			}
		}
	}
}

func (v *validator) layout(path, name string, l *Layout, served map[frame.Shape]string, used map[string]bool) {
	if !partNameRe.MatchString(name) {
		v.c.errorf(path, "%q is not a layout name, which is lowercase letters, digits, underscores, and hyphens", name)
	}
	if len(l.Serves) == 0 {
		v.c.errorf(path+".serves", "the layout serves no shape")
	}
	for i, sh := range l.Serves {
		sp := fmt.Sprintf("%s.serves[%d]", path, i)
		if !frame.KnownRole(sh.Role) {
			v.c.errorf(sp+".role", "%q is not a role", sh.Role)
		}
		if !frame.KnownKind(sh.Kind) {
			v.c.errorf(sp+".kind", "%q is not a kind", sh.Kind)
		}
		if other, ok := served[sh]; ok && other != name {
			v.c.errorf(sp, "layouts %q and %q both serve %s", other, name, sh)
		}
		served[sh] = name
	}
	v.canvas(path+".canvas", l.Canvas)

	if len(l.Stack) == 0 {
		v.c.errorf(path+".stack", "the stack is empty")
	}
	if len(l.Stack) > MaxStack {
		v.c.errorf(path+".stack", "there are %d entries, over the limit of %d", len(l.Stack), MaxStack)
	}
	entries := map[string]bool{}
	for i := range l.Stack {
		e := &l.Stack[i]
		if e.ID == "" {
			e.ID = e.Part
		}
		v.entry(fmt.Sprintf("%s.stack[%d]", path, i), e, entries, used)
	}

	if len(l.TextBoxes) > MaxTextBoxes {
		v.c.errorf(path+".textBoxes", "there are %d, over the limit of %d", len(l.TextBoxes), MaxTextBoxes)
	}
	for _, bn := range sortedKeys(l.TextBoxes) {
		b := l.TextBoxes[bn]
		v.textBox(path+".textBoxes."+bn, bn, &b, l, entries)
		l.TextBoxes[bn] = b
	}
	if len(l.Art) == 0 {
		v.c.warnf(path+".art", "the layout has no art window")
	}
	for i, a := range l.Art {
		ap := fmt.Sprintf("%s.art[%d]", path, i)
		v.size(ap, a.X, a.Y, a.Width, a.Height)
		if a.After != "" && !entries[a.After] {
			v.c.errorf(ap+".after", "no stack entry has the id %q", a.After)
		}
		v.face(ap+".face", a.Face)
		v.rotate(ap+".rotate", a.Rotate)
	}
	for _, sn := range sortedKeys(l.Symbols) {
		v.symbol(path+".symbols."+sn, sn, l.Symbols[sn])
	}
	v.sections(path+".sections", l, entries, used)
	// A layout's boxes refer to each other, so those references are checked once
	// every box in it is known
	for _, bn := range sortedKeys(l.TextBoxes) {
		v.boxReferences(path+".textBoxes."+bn, bn, l.TextBoxes[bn], l, entries, used)
	}
}

func (v *validator) canvas(path string, c Canvas) {
	if c.Width <= 0 || c.Height <= 0 || c.Width > MaxDimension || c.Height > MaxDimension {
		v.c.errorf(path, "%d by %d is not a size from 1 to %d on a side", c.Width, c.Height, MaxDimension)
	}
	if c.DPI < 0 || c.DPI > 10000 {
		v.c.errorf(path+".dpi", "%d is not a resolution", c.DPI)
	}
	v.rotate(path+".rotate", c.Rotate)
}

func (v *validator) rotate(path string, deg int) {
	if deg < 0 || deg >= 360 || deg%90 != 0 {
		v.c.errorf(path, "%d is not 0, 90, 180, or 270", deg)
	}
}

func (v *validator) size(path string, x, y, w, h int) {
	if w <= 0 || h <= 0 || w > MaxDimension || h > MaxDimension {
		v.c.errorf(path, "%d by %d is not a size from 1 to %d on a side", w, h, MaxDimension)
	}
	if x < -MaxDimension || x > MaxDimension || y < -MaxDimension || y > MaxDimension {
		v.c.errorf(path, "(%d, %d) is off the canvas", x, y)
	}
}

func (v *validator) face(path, f string) {
	switch f {
	case "", "self", "first", "second", "other":
	default:
		v.c.errorf(path, "%q is not a face, which is self, first, second, or other", f)
	}
}

func (v *validator) entry(path string, e *Entry, ids map[string]bool, used map[string]bool) {
	if !partNameRe.MatchString(e.ID) {
		v.c.errorf(path+".id", "%q is not an entry id, which is lowercase letters, digits, underscores, and hyphens", e.ID)
	} else if ids[e.ID] {
		v.c.errorf(path+".id", "the id %q is used twice in the stack", e.ID)
	}
	ids[e.ID] = true
	if _, ok := v.m.Parts[e.Part]; !ok {
		v.c.errorf(path+".part", "no part is named %q", e.Part)
	} else {
		used[e.Part] = true
	}
	v.condition(path+".condition", e.Condition)
	v.face(path+".face", e.Face)
	if e.At != nil && (e.At.X < -MaxDimension || e.At.X > MaxDimension || e.At.Y < -MaxDimension || e.At.Y > MaxDimension) {
		v.c.errorf(path+".at", "(%d, %d) is off the canvas", e.At.X, e.At.Y)
	}
	if e.Mirror != nil {
		v.condition(path+".mirror.condition", e.Mirror.Condition)
		if e.Mirror.Width < 0 || e.Mirror.Height < 0 || e.Mirror.X < 0 || e.Mirror.Y < 0 {
			v.c.errorf(path+".mirror", "the region has a negative side or corner")
		}
	}
	if _, err := ParseMode(e.Mode); err != nil {
		v.c.errorf(path+".mode", "%v", err)
	}
	switch {
	case e.Opacity != nil && e.OpacityFeature != "":
		v.c.errorf(path, "opacity and opacityFeature are both set, and only one can be")
	case e.Opacity != nil && (*e.Opacity < 0 || *e.Opacity > 1):
		v.c.errorf(path+".opacity", "%g is outside 0 to 1", *e.Opacity)
	case e.OpacityFeature != "":
		f, ok := v.m.Features[e.OpacityFeature]
		switch {
		case !ok:
			v.c.errorf(path+".opacityFeature", "the template declares no feature named %q", e.OpacityFeature)
		case f.Type != "range" || f.Min < 0 || f.Max > 1:
			v.c.errorf(path+".opacityFeature", "%q is not a range feature within 0 to 1", e.OpacityFeature)
		}
	}
	v.rotate(path+".rotate", e.Rotate)
}

// condition compiles a condition and checks each feature it refers to against the
// features the template declares
func (v *validator) condition(path, expr string) {
	cond, err := frame.Compile(expr)
	if err != nil {
		v.c.errorf(path, "%v", err)
		return
	}
	for _, ref := range cond.FeatureRefs() {
		f, ok := v.m.Features[ref.Name]
		if !ok {
			v.c.errorf(path, "%q refers to the feature %q, which the template does not declare", expr, ref.Name)
			continue
		}
		if msg := featureRefProblem(f, ref); msg != "" {
			v.c.errorf(path, "%q: %s", expr, msg)
		}
	}
}

// featureRefProblem says why a reference cannot be made to a feature, or "" when
// it can
func featureRefProblem(f Feature, ref frame.FeatureRef) string {
	switch {
	case ref.Op == "":
		if f.Type != "bool" {
			return fmt.Sprintf("the feature %q is a %s, so it cannot stand alone", ref.Name, f.Type)
		}
	case ref.Op == "=":
		switch f.Type {
		case "bool":
			if ref.Value != "true" && ref.Value != "false" {
				return fmt.Sprintf("the feature %q is a bool, which is true or false and not %q", ref.Name, ref.Value)
			}
		case "choice":
			if !slices.Contains(f.Values, ref.Value) {
				return fmt.Sprintf("%q is not a value of the feature %q", ref.Value, ref.Name)
			}
		case "range":
			if !ref.IsNumber {
				return fmt.Sprintf("the feature %q is a range, which compares with a number and not %q", ref.Name, ref.Value)
			}
		}
	default:
		if f.Type != "range" {
			return fmt.Sprintf("the feature %q is a %s, and only a range compares with %s", ref.Name, f.Type, ref.Op)
		}
	}
	return ""
}

func (v *validator) textBox(path, name string, b *TextBox, l *Layout, entries map[string]bool) {
	if !boxNameRe.MatchString(name) {
		v.c.errorf(path, "%q is not a text box name, which is letters, digits, and underscores", name)
	}
	if content := b.Content(name); !boxNameRe.MatchString(content) {
		v.c.errorf(path+".draws", "%q is not something a box draws", content)
	} else if !slices.Contains(card.TextBoxNames(), content) {
		v.c.warnf(path, "the engine fills no text into a box that draws %q", content)
	}
	v.condition(path+".condition", b.Condition)
	v.boxFields(path, b)
	for i := range b.When {
		o := &b.When[i]
		op := fmt.Sprintf("%s.when[%d]", path, i)
		if strings.TrimSpace(o.Condition) == "" {
			v.c.errorf(op+".condition", "an override with no condition always applies, so write it on the box")
		}
		v.condition(op+".condition", o.Condition)
		merged, err := v.mergeOverride(*b, *o)
		if err != nil {
			v.c.errorf(op, "%v", err)
			continue
		}
		v.boxFields(op, &merged)
		o.Box = &merged
	}
}

// boxFields checks what a box holds that does not refer to the rest of its layout
func (v *validator) boxFields(path string, b *TextBox) {
	v.size(path+".rect", b.Rect.X, b.Rect.Y, b.Rect.Width, b.Rect.Height)
	v.textStyle(path+".text", b.Text)
	if b.Flow.ClearGap < 0 {
		v.c.errorf(path+".flow.clearGap", "the gap cannot be negative")
	}
	v.face(path+".face", b.Face)
	switch b.Space {
	case "", "output":
	default:
		v.c.errorf(path+".space", "%q is not empty or \"output\"", b.Space)
	}
	v.rotate(path+".rotate", b.Rotate)
}

// textStyle checks how a box's text looks
func (v *validator) textStyle(tp string, t TextStyle) {
	if t.FontSize <= 0 {
		v.c.errorf(tp+".fontSize", "%g is not a size above 0", t.FontSize)
	}
	if t.MinFontSize < 0 || t.MinFontSize > t.FontSize {
		v.c.errorf(tp+".minFontSize", "%g is not from 0 to the font size %g", t.MinFontSize, t.FontSize)
	}
	if t.Font != "" {
		_, standard := fonts.ParseRole(t.Font)
		if _, defined := v.m.Roles[t.Font]; !standard && !defined {
			v.c.errorf(tp+".font", "%q is not a font role, which is a standard one or one the template defines in roles", t.Font)
		}
	}
	if t.Color != "" && !colorRe.MatchString(t.Color) {
		v.c.errorf(tp+".color", "%q is not a color like #RRGGBB", t.Color)
	}
	switch t.Align {
	case "", "left", "center", "right":
	default:
		v.c.errorf(tp+".align", "%q is not left, center, or right", t.Align)
	}
	switch t.VAlign {
	case "", "top", "center", "bottom", "baseline", "cap-top", "cap-center":
	default:
		v.c.errorf(tp+".vAlign", "%q is not top, center, bottom, baseline, cap-top, or cap-center", t.VAlign)
	}
	if t.LineSpacing < 0 || t.PaddingX < 0 || t.PaddingY < 0 {
		v.c.errorf(tp, "line spacing and padding cannot be negative")
	}
	if s := t.Shadow; s != nil {
		if s.Distance < 0 || s.Opacity < 0 || s.Opacity > 1 {
			v.c.errorf(tp+".shadow", "the distance cannot be negative and the opacity is from 0 to 1")
		}
		if s.Color != "" && !colorRe.MatchString(s.Color) {
			v.c.errorf(tp+".shadow.color", "%q is not a color like #RRGGBB", s.Color)
		}
	}
}

// mergeOverride merges an override's patch over a box and reads the result back,
// which fails on a field a text box does not have
func (v *validator) mergeOverride(base TextBox, o Override) (TextBox, error) {
	if o.Patch == nil {
		return base, nil
	}
	plain := base
	plain.When = nil
	doc, err := json.Marshal(plain)
	if err != nil {
		return TextBox{}, err
	}
	merged, err := MergePatch(doc, o.Patch)
	if err != nil {
		return TextBox{}, err
	}
	var out TextBox
	if err := decodeStrict(merged, &out); err != nil {
		return TextBox{}, fmt.Errorf("the override does not fit a text box: %v", err)
	}
	if len(out.When) > 0 {
		return TextBox{}, fmt.Errorf("an override cannot hold overrides of its own")
	}
	return out, nil
}

// boxReferences checks the references a box makes to the rest of its layout
func (v *validator) boxReferences(path, name string, b TextBox, l *Layout, entries, used map[string]bool) {
	check := func(p string, f Flow) {
		if f.ClearOf != "" {
			if _, ok := l.TextBoxes[f.ClearOf]; !ok || f.ClearOf == name {
				v.c.errorf(p+".clearOf", "%q is not another text box in the layout", f.ClearOf)
			}
		}
		if f.AvoidLayer != "" && !entries[f.AvoidLayer] {
			v.c.errorf(p+".avoidLayer", "no stack entry has the id %q", f.AvoidLayer)
		}
		if d := f.Duck; d != nil {
			if !entries[d.Layer] {
				v.c.errorf(p+".duck.layer", "no stack entry has the id %q", d.Layer)
			}
			if _, ok := l.TextBoxes[d.Row]; !ok || d.Row == name {
				v.c.errorf(p+".duck.row", "%q is not another text box in the layout", d.Row)
			}
		}
		if f.Divider != "" {
			if _, ok := v.m.Parts[f.Divider]; !ok {
				v.c.errorf(p+".divider", "no part is named %q", f.Divider)
			}
			// A divider is drawn by its box and not by the stack, which still uses it
			used[f.Divider] = true
		}
	}
	check(path+".flow", b.Flow)
	for i, o := range b.When {
		if o.Box != nil {
			check(fmt.Sprintf("%s.when[%d].flow", path, i), o.Box.Flow)
		}
	}
}

func (v *validator) symbol(path, name string, s Symbol) {
	if name != "set" {
		v.c.errorf(path, "%q is not a symbol the engine draws, which is only \"set\"", name)
	}
	v.size(path, s.X, s.Y, s.Width, s.Height)
	switch s.Align {
	case "", "left", "center", "right":
	default:
		v.c.errorf(path+".align", "%q is not left, center, or right", s.Align)
	}
	switch s.VAlign {
	case "", "top", "center", "bottom":
	default:
		v.c.errorf(path+".vAlign", "%q is not top, center, or bottom", s.VAlign)
	}
	if s.Scale < 0 || s.Scale > 1 {
		v.c.errorf(path+".scale", "%g is outside 0 to 1", s.Scale)
	}
	v.condition(path+".condition", s.Condition)
	v.face(path+".face", s.Face)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
