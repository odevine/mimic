// Package draftedit applies the edits the editor makes to a draft's files. An edit
// is an operation that names what it changes by layout, kind and key, and gives its
// numbers in the finished image's own pixels, the ones the person sees. The package
// turns them into the layout's own coordinates, including its rotation, changes the
// numbers in the layout file and leaves the rest of the file as it was. Each
// operation comes back with the operation that undoes it
package draftedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"path"
	"regexp"
	"strconv"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/jsonedit"
)

// What an operation changes
const (
	KindLayer  = "layer"
	KindText   = "text"
	KindArt    = "art"
	KindSymbol = "symbol"
	// KindCanvas is a layout's canvas, KindPart a part of the template, and
	// KindTemplate the template's own fields
	KindCanvas   = "canvas"
	KindPart     = "part"
	KindTemplate = "template"
)

// The operations
const (
	OpMove    = "move"
	OpResize  = "resize"
	OpReorder = "reorder"
	// OpSet puts a value at a path inside an element, and OpUnset takes it out
	OpSet   = "set"
	OpUnset = "unset"
	// OpReplace writes a whole file
	OpReplace = "replace"
)

// Rect is a position and size in the finished image
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (r Rect) image() image.Rectangle { return image.Rect(r.X, r.Y, r.X+r.Width, r.Y+r.Height) }

func fromImage(r image.Rectangle) Rect {
	return Rect{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy()}
}

// Op is one edit. Key names a stack entry by its id, or its part's name when it
// has none, a text box or symbol by its key, and an art window by its index
type Op struct {
	Op     string `json:"op"`
	Layout string `json:"layout"`
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	// DX and DY are how far a move goes, in the finished image's pixels
	DX int `json:"dx,omitempty"`
	DY int `json:"dy,omitempty"`
	// Rect is where a resize puts the element, in the finished image's pixels
	Rect *Rect `json:"rect,omitempty"`
	// To is the place in the stack a reorder moves an entry to, where 0 is the
	// bottom
	To int `json:"to,omitempty"`
	// Path names a field inside the element by its keys, such as ["text",
	// "fontSize"], for a set or an unset, and Value is what a set puts there as JSON.
	// A set of an element's numbers is in the layout's own canvas, unlike a move
	Path  []string        `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
	// File and Content are the file a replace writes and what it holds. A replace
	// is a batch of its own
	File    string `json:"file,omitempty"`
	Content string `json:"content,omitempty"`
}

// ErrExtends is the refusal for a layout that builds on another, whose numbers are
// not all in its own file
var ErrExtends = errors.New("this layout extends another one, so its positions are not in its own file")

// Result is what a batch of operations changed
type Result struct {
	// Files are the files that changed, by their path in the draft
	Files map[string][]byte
	// Inverse undoes the batch when applied in order
	Inverse []Op
}

var layoutName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Apply carries out the operations in order against the draft's files and returns
// the files they change. Nothing is written, so a batch with a bad operation
// changes nothing
func Apply(fsys fs.FS, ops []Op) (*Result, error) {
	w := &work{fsys: fsys, layouts: map[string]*layout{}}
	res := &Result{Files: map[string][]byte{}}
	for _, op := range ops {
		if op.Op == OpReplace && len(ops) > 1 {
			return nil, errors.New("a whole file is replaced on its own, not with other changes")
		}
	}
	var inverse []Op
	for i, op := range ops {
		inv, err := w.apply(op)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", describe(op, i), err)
		}
		inverse = append(inverse, inv)
	}
	for i, j := 0, len(inverse)-1; i < j; i, j = i+1, j-1 {
		inverse[i], inverse[j] = inverse[j], inverse[i]
	}
	res.Inverse = inverse
	for _, l := range w.layouts {
		if l.changed {
			res.Files[l.path] = l.root.Marshal()
		}
	}
	if w.sourceChanged {
		res.Files[sourceFile] = w.source.Marshal()
	}
	for name, data := range w.replaced {
		res.Files[name] = data
	}
	return res, nil
}

// Editable reports whether a layout's numbers can be changed here, which they cannot
// when it builds on another layout. It says why with ErrExtends
func Editable(fsys fs.FS, layoutName string) error {
	w := &work{fsys: fsys, layouts: map[string]*layout{}}
	_, err := w.layout(layoutName)
	return err
}

func describe(op Op, i int) string {
	if op.Key == "" {
		return fmt.Sprintf("operation %d (%s)", i+1, op.Op)
	}
	return fmt.Sprintf("%s of %s %q", op.Op, op.Kind, op.Key)
}

const sourceFile = "template.json"

type work struct {
	fsys    fs.FS
	layouts map[string]*layout
	// source is template.json, read when an edit needs it
	source        *jsonedit.Node
	sourceChanged bool
	// replaced is the files a replace wrote
	replaced map[string][]byte
}

type layout struct {
	path    string
	root    *jsonedit.Node
	changed bool
	// authored is the layout's canvas, rotate is its turn, and delivered is the
	// size of the finished image
	authored, delivered image.Point
	rotate              int
}

func (w *work) layout(name string) (*layout, error) {
	if l, ok := w.layouts[name]; ok {
		return l, nil
	}
	if !layoutName.MatchString(name) {
		return nil, fmt.Errorf("%q is not a layout", name)
	}
	file := path.Join("layouts", name+".json")
	raw, err := fs.ReadFile(w.fsys, file)
	if err != nil {
		return nil, fmt.Errorf("the draft has no layout %q", name)
	}
	root, err := jsonedit.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", file, err)
	}
	if root.Kind() != jsonedit.Object {
		return nil, fmt.Errorf("%s is not an object", file)
	}
	if root.Field("extends") != nil || root.Field("patch") != nil {
		return nil, ErrExtends
	}
	l := &layout{path: file, root: root}
	cw, okw := root.Path("canvas", "width").Int()
	ch, okh := root.Path("canvas", "height").Int()
	if !okw || !okh || cw <= 0 || ch <= 0 {
		return nil, fmt.Errorf("%s has no canvas size", file)
	}
	l.authored = image.Pt(cw, ch)
	l.rotate, _ = root.Path("canvas", "rotate").Int()
	l.rotate = ((l.rotate % 360) + 360) % 360
	l.delivered = l.authored
	if l.rotate == 90 || l.rotate == 270 {
		l.delivered = image.Pt(ch, cw)
	}
	w.layouts[name] = l
	return l, nil
}

// authoredDelta turns a movement in the finished image into one in the layout's
// own canvas, which differs when the layout turns the card
func authoredDelta(dx, dy, rotate int) (int, int) {
	switch rotate {
	case 90:
		return dy, -dx
	case 180:
		return -dx, -dy
	case 270:
		return -dy, dx
	}
	return dx, dy
}

func (w *work) apply(op Op) (Op, error) {
	switch op.Op {
	case OpReplace:
		return w.replace(op)
	case OpSet, OpUnset:
		return w.change(op)
	}
	l, err := w.layout(op.Layout)
	if err != nil {
		return Op{}, err
	}
	switch op.Op {
	case OpMove:
		return w.move(l, op)
	case OpResize:
		return w.resize(l, op)
	case OpReorder:
		return w.reorder(l, op)
	}
	return Op{}, fmt.Errorf("%q is not an operation", op.Op)
}

// target is the node that holds an element's numbers, and whether they are in the
// finished image's canvas already
type target struct {
	node   *jsonedit.Node
	output bool
}

func (w *work) find(l *layout, op Op) (target, error) {
	switch op.Kind {
	case KindText:
		box := l.root.Path("textBoxes", op.Key)
		if box == nil || box.Field("rect") == nil {
			return target{}, fmt.Errorf("the layout has no text box %q", op.Key)
		}
		space, _ := box.Field("space").String()
		return target{node: box.Field("rect"), output: space == template.SpaceOutput}, nil
	case KindArt:
		i, err := strconv.Atoi(op.Key)
		window := l.root.Field("art").Item(i)
		if err != nil || window == nil {
			return target{}, fmt.Errorf("the layout has no art window %q", op.Key)
		}
		return target{node: window}, nil
	case KindSymbol:
		sym := l.root.Path("symbols", op.Key)
		if sym == nil {
			return target{}, fmt.Errorf("the layout has no symbol %q", op.Key)
		}
		return target{node: sym}, nil
	}
	return target{}, fmt.Errorf("%q is not something that can be changed", op.Kind)
}

func readRect(n *jsonedit.Node) (image.Rectangle, error) {
	x, okx := n.Field("x").Int()
	y, oky := n.Field("y").Int()
	w, okw := n.Field("width").Int()
	h, okh := n.Field("height").Int()
	if !okx || !oky || !okw || !okh {
		return image.Rectangle{}, errors.New("its position and size are not whole numbers")
	}
	return image.Rect(x, y, x+w, y+h), nil
}

func writeRect(n *jsonedit.Node, r image.Rectangle) {
	_ = n.Set("x", jsonedit.NewInt(r.Min.X))
	_ = n.Set("y", jsonedit.NewInt(r.Min.Y))
	_ = n.Set("width", jsonedit.NewInt(r.Dx()))
	_ = n.Set("height", jsonedit.NewInt(r.Dy()))
}

// stackEntry finds an entry of the stack by its id, or its part's name when it has
// none, and returns its place
func stackEntry(l *layout, key string) (*jsonedit.Node, int, error) {
	stack := l.root.Field("stack")
	for i := range stack.Len() {
		e := stack.Item(i)
		id, hasID := e.Field("id").String()
		if !hasID {
			id, _ = e.Field("part").String()
		}
		if id == key {
			return e, i, nil
		}
	}
	return nil, 0, fmt.Errorf("the layout's stack has no entry %q", key)
}

// partOrigin is where a part's images sit in the canvas, which is where an entry
// that places nothing itself draws them
func (w *work) partOrigin(part string) (image.Point, error) {
	src, err := w.sourceNode()
	if err != nil {
		return image.Point{}, err
	}
	o := src.Path("parts", part, "origin")
	if o == nil {
		return image.Point{}, nil
	}
	x, okx := o.Field("x").Int()
	y, oky := o.Field("y").Int()
	if !okx || !oky {
		return image.Point{}, fmt.Errorf("the origin of part %q is not whole numbers", part)
	}
	return image.Pt(x, y), nil
}

// sourceNode is template.json as a tree, read the first time an edit needs it
func (w *work) sourceNode() (*jsonedit.Node, error) {
	if w.source == nil {
		raw, err := fs.ReadFile(w.fsys, sourceFile)
		if err != nil {
			return nil, errors.New("the draft has no template.json")
		}
		src, err := jsonedit.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("template.json is not valid JSON: %w", err)
		}
		if src.Kind() != jsonedit.Object {
			return nil, errors.New("template.json is not an object")
		}
		w.source = src
	}
	return w.source, nil
}

func (w *work) move(l *layout, op Op) (Op, error) {
	inv := Op{Op: OpMove, Layout: op.Layout, Kind: op.Kind, Key: op.Key, DX: -op.DX, DY: -op.DY}
	if op.Kind == KindLayer {
		e, _, err := stackEntry(l, op.Key)
		if err != nil {
			return Op{}, err
		}
		part, _ := e.Field("part").String()
		at := image.Point{}
		if cur := e.Field("at"); cur != nil {
			x, okx := cur.Field("x").Int()
			y, oky := cur.Field("y").Int()
			if !okx || !oky {
				return Op{}, errors.New("its position is not whole numbers")
			}
			at = image.Pt(x, y)
		} else if at, err = w.partOrigin(part); err != nil {
			return Op{}, err
		}
		dx, dy := authoredDelta(op.DX, op.DY, l.rotate)
		node := e.Field("at")
		if node == nil {
			node = jsonedit.NewObject()
			_ = e.Set("at", node)
		}
		_ = node.Set("x", jsonedit.NewInt(at.X+dx))
		_ = node.Set("y", jsonedit.NewInt(at.Y+dy))
		l.changed = true
		return inv, nil
	}
	t, err := w.find(l, op)
	if err != nil {
		return Op{}, err
	}
	r, err := readRect(t.node)
	if err != nil {
		return Op{}, err
	}
	dx, dy := op.DX, op.DY
	if !t.output {
		dx, dy = authoredDelta(dx, dy, l.rotate)
	}
	writeRect(t.node, r.Add(image.Pt(dx, dy)))
	l.changed = true
	return inv, nil
}

func (w *work) resize(l *layout, op Op) (Op, error) {
	if op.Kind == KindLayer {
		return Op{}, errors.New("a layer cannot be resized, since its image is drawn at its own size")
	}
	if op.Rect == nil || op.Rect.Width < 1 || op.Rect.Height < 1 {
		return Op{}, errors.New("the new size must be at least one pixel each way")
	}
	t, err := w.find(l, op)
	if err != nil {
		return Op{}, err
	}
	old, err := readRect(t.node)
	if err != nil {
		return Op{}, err
	}
	next, was := op.Rect.image(), old
	if !t.output {
		next = template.RotateRect(next, l.delivered.X, l.delivered.Y, 360-l.rotate)
		was = template.RotateRect(old, l.authored.X, l.authored.Y, l.rotate)
	}
	writeRect(t.node, next)
	l.changed = true
	back := fromImage(was)
	return Op{Op: OpResize, Layout: op.Layout, Kind: op.Kind, Key: op.Key, Rect: &back}, nil
}

func (w *work) reorder(l *layout, op Op) (Op, error) {
	if op.Kind != KindLayer {
		return Op{}, errors.New("only a layer has a place in the stack")
	}
	_, from, err := stackEntry(l, op.Key)
	if err != nil {
		return Op{}, err
	}
	if err := l.root.Field("stack").MoveItem(from, op.To); err != nil {
		return Op{}, fmt.Errorf("there is no place %d in the stack", op.To)
	}
	l.changed = true
	return Op{Op: OpReorder, Layout: op.Layout, Kind: op.Kind, Key: op.Key, To: from}, nil
}

var fieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// maxPath is how deep a field can be, which is deeper than any the manifest has
const maxPath = 4

// element is the node an element's fields are in, and what marks it changed
func (w *work) element(op Op) (*jsonedit.Node, func(), error) {
	switch op.Kind {
	case KindTemplate:
		src, err := w.sourceNode()
		return src, func() { w.sourceChanged = true }, err
	case KindPart:
		src, err := w.sourceNode()
		if err != nil {
			return nil, nil, err
		}
		part := src.Path("parts", op.Key)
		if part == nil || part.Kind() != jsonedit.Object {
			return nil, nil, fmt.Errorf("the template has no part %q", op.Key)
		}
		return part, func() { w.sourceChanged = true }, nil
	}
	l, err := w.layout(op.Layout)
	if err != nil {
		return nil, nil, err
	}
	changed := func() { l.changed = true }
	var node *jsonedit.Node
	switch op.Kind {
	case KindCanvas:
		node = l.root.Field("canvas")
	case KindLayer:
		node, _, err = stackEntry(l, op.Key)
		if err != nil {
			return nil, nil, err
		}
	case KindText:
		node = l.root.Path("textBoxes", op.Key)
	case KindArt:
		if i, err := strconv.Atoi(op.Key); err == nil {
			node = l.root.Field("art").Item(i)
		}
	case KindSymbol:
		node = l.root.Path("symbols", op.Key)
	default:
		return nil, nil, fmt.Errorf("%q is not something that can be changed", op.Kind)
	}
	if node == nil || node.Kind() != jsonedit.Object {
		return nil, nil, fmt.Errorf("the layout has no %s %q", op.Kind, op.Key)
	}
	return node, changed, nil
}

// change sets or unsets a field of an element, and returns the operation that puts it
// back
func (w *work) change(op Op) (Op, error) {
	if len(op.Path) == 0 || len(op.Path) > maxPath {
		return Op{}, fmt.Errorf("a field is named by 1 to %d keys", maxPath)
	}
	for _, name := range op.Path {
		if !fieldName.MatchString(name) {
			return Op{}, fmt.Errorf("%q is not the name of a field", name)
		}
	}
	node, changed, err := w.element(op)
	if err != nil {
		return Op{}, err
	}
	back := Op{Layout: op.Layout, Kind: op.Kind, Key: op.Key, Path: op.Path}
	var old *jsonedit.Node
	if op.Op == OpSet {
		v, err := jsonedit.Parse(op.Value)
		if err != nil {
			return Op{}, fmt.Errorf("the value is not JSON: %w", err)
		}
		if old, err = node.SetPath(op.Path, v); err != nil {
			return Op{}, err
		}
	} else {
		old = node.DeletePath(op.Path)
	}
	changed()
	if old == nil {
		back.Op = OpUnset
	} else {
		back.Op, back.Value = OpSet, json.RawMessage(old.Compact())
	}
	return back, nil
}

var layoutFile = regexp.MustCompile(`^layouts/[A-Za-z0-9][A-Za-z0-9_-]*\.json$`)

// replace writes a whole file, as the JSON editor does. The file has to be one of the
// draft's, and what it is to hold has to be a JSON object, but it is kept as written
func (w *work) replace(op Op) (Op, error) {
	if op.File != sourceFile && !layoutFile.MatchString(op.File) {
		return Op{}, fmt.Errorf("%q is not a file of the template that can be written", op.File)
	}
	was, err := fs.ReadFile(w.fsys, op.File)
	if err != nil {
		return Op{}, fmt.Errorf("the draft has no file %s", op.File)
	}
	n, err := jsonedit.Parse([]byte(op.Content))
	if err != nil {
		return Op{}, fmt.Errorf("%s is not valid JSON: %w", op.File, err)
	}
	if n.Kind() != jsonedit.Object {
		return Op{}, fmt.Errorf("%s is not a JSON object", op.File)
	}
	if w.replaced == nil {
		w.replaced = map[string][]byte{}
	}
	w.replaced[op.File] = []byte(op.Content)
	return Op{Op: OpReplace, File: op.File, Content: string(was)}, nil
}
