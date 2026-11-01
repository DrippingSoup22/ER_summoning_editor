package gio

// The whole window driven as a player drives it: through Gio's own router,
// finding a control by what it says (the semantic tree), clicking where it is
// drawn and typing as the platform types. A test that calls setRole cannot
// see a button that moves under the pointer, or one that looks open and is
// not; this can.

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

type hands struct {
	t      *testing.T
	state  *State
	router *input.Router
	now    time.Time
}

func newHands(t *testing.T, state *State) *hands {
	h := &hands{t: t, state: state, router: new(input.Router), now: time.Unix(1000, 0)}
	h.frame()
	return h
}

func (h *hands) frame() {
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1280, 800)), Source: h.router.Source(), Now: h.now}
	h.state.Layout(gtx, h.state.Theme())
	h.router.Frame(gtx.Ops)
}

// wait lets time pass, and draws the frame it brings.
func (h *hands) wait(d time.Duration) {
	h.now = h.now.Add(d)
	h.frame()
}

// controls is every control on screen that takes a click, or with editors
// every text field, and what it says, each cut to what its parents show.
func (h *hands) controls(editors bool) map[image.Rectangle]string {
	out := map[image.Rectangle]string{}
	for _, one := range h.inOrder(editors) {
		out[one.bounds] = one.says
	}
	return out
}

// control is one control on screen and what it says.
type control struct {
	bounds image.Rectangle
	says   string
}

// inOrder is controls in the order they are drawn, the last on top: a dialog
// is drawn after what it covers.
func (h *hands) inOrder(editors bool) []control {
	var out []control
	seen := map[image.Rectangle]bool{}
	var walk func(node input.SemanticNode, shown image.Rectangle)
	walk = func(node input.SemanticNode, shown image.Rectangle) {
		bounds := node.Desc.Bounds.Intersect(shown)
		wanted := node.Desc.Gestures&input.ClickGesture != 0
		if editors {
			wanted = node.Desc.Class == semantic.Editor
		}
		// A drag source over a card shares its bounds and says nothing; the
		// card is the one that says what it is.
		if wanted && bounds.Dx() > 2 && bounds.Dy() > 2 && !seen[bounds] {
			says := said(node)
			seen[bounds] = says != ""
			out = append(out, control{bounds, says})
		}
		for _, child := range node.Children {
			walk(child, bounds)
		}
	}
	if tree := h.router.AppendSemantics(nil); len(tree) > 0 {
		walk(tree[0], image.Rect(0, 0, 1280, 800))
	}
	return out
}

// screen is everything the window shows with words, and where: what a
// player sees, to compare before and after a gesture.
func (h *hands) screen() []string {
	var out []string
	var walk func(node input.SemanticNode)
	walk = func(node input.SemanticNode) {
		if node.Desc.Label != "" {
			out = append(out, fmt.Sprintf("%v %s", node.Desc.Bounds, node.Desc.Label))
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	if tree := h.router.AppendSemantics(nil); len(tree) > 0 {
		walk(tree[0])
	}
	return out
}

// said is every label under a node.
func said(node input.SemanticNode) string {
	var words []string
	if node.Desc.Label != "" {
		words = append(words, node.Desc.Label)
	}
	for _, child := range node.Children {
		if label := said(child); label != "" {
			words = append(words, label)
		}
	}
	return strings.Join(words, " ")
}

// find is where the control that says exactly label is drawn: the smallest,
// since a list holding that one control says the same, and of two alike the
// one drawn last - the one on top, which is where a hand's click lands.
func (h *hands) find(label string) (f32.Point, bool) {
	var best image.Rectangle
	for _, one := range h.inOrder(false) {
		if one.says == label && (best.Empty() || one.bounds.Dx()*one.bounds.Dy() <= best.Dx()*best.Dy()) {
			best = one.bounds
		}
	}
	return layout.FPt(best.Min.Add(best.Max).Div(2)), !best.Empty()
}

// described is where the control a screen reader describes as desc is drawn:
// a switch, which carries its name as a description rather than a label.
func (h *hands) described(desc string) (f32.Point, bool) {
	var found image.Rectangle
	var walk func(node input.SemanticNode)
	walk = func(node input.SemanticNode) {
		if node.Desc.Description == desc && node.Desc.Gestures&input.ClickGesture != 0 {
			found = node.Desc.Bounds
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	if tree := h.router.AppendSemantics(nil); len(tree) > 0 {
		walk(tree[0])
	}
	return layout.FPt(found.Min.Add(found.Max).Div(2)), !found.Empty()
}

// clickAt clicks as a hand does: the press and the release in two frames.
func (h *hands) clickAt(at f32.Point) {
	h.router.Queue(
		pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: at},
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at})
	h.frame()
	h.router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at})
	h.frame()
}

// click clicks the control that says label.
func (h *hands) click(label string) {
	h.t.Helper()
	at, ok := h.find(label)
	if !ok {
		h.t.Fatalf("no control says %q", label)
	}
	h.clickAt(at)
}

// point moves the pointer onto the control that says label.
func (h *hands) point(label string) {
	h.t.Helper()
	at, ok := h.find(label)
	if !ok {
		h.t.Fatalf("no control says %q", label)
	}
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: at})
	h.frame()
}

// type_ types as the platform does: each character replaces the field's
// selection, and the caret goes after it.
func (h *hands) type_(text string) {
	selection := h.router.EditorState().Selection.Range
	for _, r := range text {
		h.router.Queue(key.EditEvent{Range: selection, Text: string(r)})
		at := min(selection.Start, selection.End) + 1
		selection = key.Range{Start: at, End: at}
		h.router.Queue(key.SelectionEvent(selection))
		h.frame()
	}
}

func (h *hands) press(name key.Name) {
	h.router.Queue(key.Event{Name: name, State: key.Press})
	h.frame()
}
