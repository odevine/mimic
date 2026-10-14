package draftedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/jsonedit"
)

// textKey is what a text box may be named, and layerID what a stack entry may be
// called, as the engine's own checks have them
var (
	textKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	layerID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

// array is a layout's list at a key, made when the layout has none
func (l *layout) array(name string) *jsonedit.Node {
	a := l.root.Field(name)
	if a == nil || a.Kind() != jsonedit.Array {
		a = jsonedit.NewArray()
		_ = l.root.Set(name, a)
	}
	return a
}

// object is a layout's object at a key, made when the layout has none
func (l *layout) object(name string) *jsonedit.Node {
	o := l.root.Field(name)
	if o == nil || o.Kind() != jsonedit.Object {
		o = jsonedit.NewObject()
		_ = l.root.Set(name, o)
	}
	return o
}

// entryID is the key a stack entry is named by
func entryID(e *jsonedit.Node) string {
	if id, ok := e.Field("id").String(); ok {
		return id
	}
	id, _ := e.Field("part").String()
	return id
}

// add puts an element into the layout. A layer's key is its id and To its place in
// the stack, a text box's key is its name, and an art window's key is the place in
// the list it goes to. Rect, when given, is where a text box or art window sits in
// the finished image, and is turned into the layout's own numbers
func (w *work) add(l *layout, op Op) (Op, error) {
	node, err := jsonedit.Parse(op.Value)
	if err != nil {
		return Op{}, fmt.Errorf("the new element is not JSON: %w", err)
	}
	if node.Kind() != jsonedit.Object {
		return Op{}, errors.New("the new element is not an object")
	}
	back := Op{Op: OpDelete, Layout: op.Layout, Kind: op.Kind, Key: op.Key}
	switch op.Kind {
	case KindLayer:
		if _, ok := node.Field("part").String(); !ok {
			return Op{}, errors.New("a layer needs a part to draw")
		}
		if _, ok := node.Field("id").String(); !ok && op.Key != entryID(node) {
			return Op{}, errors.New("a layer with no id is named for its part")
		}
		if id, ok := node.Field("id").String(); ok && id != op.Key {
			return Op{}, fmt.Errorf("the layer's id is %q and not %q", id, op.Key)
		}
		if !layerID.MatchString(op.Key) {
			return Op{}, fmt.Errorf("%q is not a name for a layer, which is lowercase letters, digits, underscores and hyphens", op.Key)
		}
		if _, _, err := stackEntry(l, op.Key); err == nil {
			return Op{}, fmt.Errorf("the stack already has an entry %q", op.Key)
		}
		stack := l.array("stack")
		if err := stack.InsertItem(op.To, node); err != nil {
			return Op{}, fmt.Errorf("there is no place %d in the stack", op.To)
		}
	case KindText:
		if !textKey.MatchString(op.Key) {
			return Op{}, fmt.Errorf("%q is not a name for a text box, which is letters, digits and underscores", op.Key)
		}
		boxes := l.object("textBoxes")
		if boxes.Field(op.Key) != nil {
			return Op{}, fmt.Errorf("the layout already has a text box %q", op.Key)
		}
		if err := w.place(l, node, op.Rect, false); err != nil {
			return Op{}, err
		}
		_ = boxes.SetBefore(op.Key, node, op.Before)
	case KindArt:
		at, err := strconv.Atoi(op.Key)
		art := l.array("art")
		if err != nil || art.InsertItem(at, node) != nil {
			return Op{}, fmt.Errorf("there is no place %q in the art windows", op.Key)
		}
		if err := w.place(l, node, op.Rect, false); err != nil {
			return Op{}, err
		}
	default:
		return Op{}, fmt.Errorf("a %s cannot be added", op.Kind)
	}
	l.changed = true
	return back, nil
}

// place writes a rectangle in the finished image into an element's own numbers. A
// text box set in the output's space already has them
func (w *work) place(l *layout, node *jsonedit.Node, r *Rect, output bool) error {
	if r == nil {
		return nil
	}
	if r.Width < 1 || r.Height < 1 {
		return errors.New("the new element must be at least one pixel each way")
	}
	target := node
	if node.Field("rect") != nil || node.Field("draws") != nil || node.Field("text") != nil {
		target = node.Field("rect")
		if target == nil {
			target = jsonedit.NewObject()
			_ = node.Set("rect", target)
		}
	}
	rect := r.image()
	if !output {
		rect = template.RotateRect(rect, l.delivered.X, l.delivered.Y, 360-l.rotate)
	}
	writeRect(target, rect)
	return nil
}

// delete takes an element out of the layout and returns the operation that puts it
// back where it was
func (w *work) delete(l *layout, op Op) (Op, error) {
	back := Op{Op: OpAdd, Layout: op.Layout, Kind: op.Kind, Key: op.Key}
	switch op.Kind {
	case KindLayer:
		e, at, err := stackEntry(l, op.Key)
		if err != nil {
			return Op{}, err
		}
		back.Value, back.To = json.RawMessage(e.Compact()), at
		if _, err := l.root.Field("stack").RemoveItem(at); err != nil {
			return Op{}, err
		}
	case KindText:
		box := l.root.Path("textBoxes", op.Key)
		if box == nil {
			return Op{}, fmt.Errorf("the layout has no text box %q", op.Key)
		}
		back.Value = json.RawMessage(box.Compact())
		back.Before = l.root.Field("textBoxes").KeyAfter(op.Key)
		l.root.Field("textBoxes").Delete(op.Key)
	case KindArt:
		at, err := strconv.Atoi(op.Key)
		window := l.root.Field("art").Item(at)
		if err != nil || window == nil {
			return Op{}, fmt.Errorf("the layout has no art window %q", op.Key)
		}
		back.Value = json.RawMessage(window.Compact())
		if _, err := l.root.Field("art").RemoveItem(at); err != nil {
			return Op{}, err
		}
	default:
		return Op{}, fmt.Errorf("a %s cannot be deleted", op.Kind)
	}
	l.changed = true
	return back, nil
}

// duplicate copies an element and moves the copy by DX and DY in the finished image.
// A layer is copied to As, just above the original, a text box to As, and an art
// window to the next place in the list
func (w *work) duplicate(l *layout, op Op) (Op, error) {
	var from *jsonedit.Node
	var add Op
	switch op.Kind {
	case KindLayer:
		e, at, err := stackEntry(l, op.Key)
		if err != nil {
			return Op{}, err
		}
		from = e
		add = Op{Op: OpAdd, Layout: op.Layout, Kind: op.Kind, Key: op.As, To: at + 1}
	case KindText:
		box := l.root.Path("textBoxes", op.Key)
		if box == nil {
			return Op{}, fmt.Errorf("the layout has no text box %q", op.Key)
		}
		from = box
		add = Op{Op: OpAdd, Layout: op.Layout, Kind: op.Kind, Key: op.As}
	case KindArt:
		at, err := strconv.Atoi(op.Key)
		window := l.root.Field("art").Item(at)
		if err != nil || window == nil {
			return Op{}, fmt.Errorf("the layout has no art window %q", op.Key)
		}
		from = window
		add = Op{Op: OpAdd, Layout: op.Layout, Kind: op.Kind, Key: strconv.Itoa(at + 1)}
	default:
		return Op{}, fmt.Errorf("a %s cannot be duplicated", op.Kind)
	}
	copied, err := jsonedit.Parse(from.Compact())
	if err != nil {
		return Op{}, err
	}
	switch op.Kind {
	case KindLayer:
		_ = copied.Set("id", jsonedit.NewString(op.As))
	case KindText:
		// A box with no "draws" draws what its key names, so the copy has to say it
		if copied.Field("draws") == nil {
			_ = copied.Set("draws", jsonedit.NewString(op.Key))
		}
	}
	add.Value = json.RawMessage(copied.Compact())
	back, err := w.add(l, add)
	if err != nil {
		return Op{}, err
	}
	if op.DX != 0 || op.DY != 0 {
		if _, err := w.move(l, Op{Op: OpMove, Layout: op.Layout, Kind: op.Kind, Key: add.Key, DX: op.DX, DY: op.DY}); err != nil {
			return Op{}, err
		}
	}
	return back, nil
}

// fileKind is what a put or a drop may touch: a PNG image anywhere in the draft, or
// a font in its font folder, and never a name that starts with a dot
func checkFile(name string) error {
	if !fs.ValidPath(name) || name == "." {
		return fmt.Errorf("%q is not a file inside the template", name)
	}
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") {
			return fmt.Errorf("%q is not a file the editor writes", name)
		}
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".png":
		return nil
	case ".ttf", ".otf":
		if path.Dir(name) == "font" {
			return nil
		}
		return fmt.Errorf("a font goes directly in the font folder, not at %q", name)
	}
	return fmt.Errorf("%q is not an image or a font", name)
}

// read is a file as the batch has it so far
func (w *work) read(name string) ([]byte, bool) {
	if data, ok := w.files[name]; ok {
		return data, data != nil
	}
	data, err := fs.ReadFile(w.fsys, name)
	return data, err == nil
}

func (w *work) stash(data []byte) (string, error) {
	if w.blobs == nil {
		return "", errors.New("this batch cannot hold a file")
	}
	return w.blobs.Put(data), nil
}

// put writes an image or font file. Its inverse puts back what was there, or drops
// the file when there was nothing
func (w *work) put(op Op) (Op, error) {
	if err := checkFile(op.File); err != nil {
		return Op{}, err
	}
	if w.blobs == nil {
		return Op{}, errors.New("this batch cannot hold a file")
	}
	data, ok := w.blobs.Get(op.Stash)
	if !ok {
		return Op{}, errors.New("the file to write is no longer held, so this cannot be redone")
	}
	var back Op
	if was, had := w.read(op.File); had {
		token, err := w.stash(was)
		if err != nil {
			return Op{}, err
		}
		back = Op{Op: OpPut, File: op.File, Stash: token}
	} else {
		back = Op{Op: OpDrop, File: op.File}
	}
	if w.files == nil {
		w.files = map[string][]byte{}
	}
	w.files[op.File] = data
	return back, nil
}

// drop removes an image or font file, and returns the operation that puts it back
func (w *work) drop(op Op) (Op, error) {
	if err := checkFile(op.File); err != nil {
		return Op{}, err
	}
	was, had := w.read(op.File)
	if !had {
		return Op{}, fmt.Errorf("the draft has no file %s", op.File)
	}
	token, err := w.stash(was)
	if err != nil {
		return Op{}, err
	}
	if w.files == nil {
		w.files = map[string][]byte{}
	}
	w.files[op.File] = nil
	return Op{Op: OpPut, File: op.File, Stash: token}, nil
}
