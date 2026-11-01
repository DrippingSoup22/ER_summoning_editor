package gio

// The catalog's marks: what a tile shows as small icons after its subtitle
//, in place of words the player had to decode -
// "caveat" said that something was off and nothing said what. Each is
// explained in the catalog's legend, the "?" beside SPIRITS (help.go), and a
// note's own words show when the pointer rests on its tile.

import (
	"image"
	"regexp"
	"slices"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	vector "golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// mark is one property a tile can show.
type mark uint8

const (
	// markStatue stands where it is summoned and never moves.
	markStatue mark = 1 << iota
	// markExtra is something the game never places: cut, unused, or an odd
	// look research found in the files.
	markExtra
	// markOnFoot rides a mount in most of its placements in the game and
	// arrives without it: no summon can be seated on one. One that rides in fewer
	// says so in a note.
	markOnFoot
	// markHurts can hurt the player who summons it.
	markHurts
	// markNote works, and differs from the game in a way its note says.
	markNote
)

// everyMarks are a card's or an entry's only when every variant under it
// has them - a card is a statue only if all of it is - and anyMarks when one
// has: a note anywhere is worth opening the card for.
const (
	everyMarks = markStatue | markExtra | markOnFoot
	anyMarks   = markHurts | markNote
)

// marksOver is a card's marks over its variants (everyMarks, anyMarks), and
// what its note mark says: the notes, where every variant has the same ones,
// else where to find them.
func marksOver(variants []variantView) (mark, []string) {
	if len(variants) == 0 {
		return 0, nil
	}
	every, any, same := everyMarks, mark(0), true
	for _, variant := range variants {
		every &= variant.Marks
		any |= variant.Marks & anyMarks
		same = same && slices.Equal(variant.Notes, variants[0].Notes)
	}
	marks := every | any
	switch {
	case marks&markNote == 0:
		return marks, nil
	case same:
		return marks, variants[0].Notes
	}
	return marks, []string{"Some of the creatures inside differ from the game: open the card to read how."}
}

// markOrder is the order the icons are drawn in, and the legend's.
var markOrder = []mark{markStatue, markExtra, markOnFoot, markHurts, markNote}

// markIcons are the icons, built once: pause for a statue, a star for an
// extra, a walker for one that arrives on foot, a triangle for a hurt, an "i"
// for a note.
var markIcons = func() map[mark]*widget.Icon {
	data := map[mark][]byte{
		markStatue: vector.AVPauseCircleOutline,
		markExtra:  vector.ToggleStarBorder,
		markOnFoot: vector.MapsDirectionsWalk,
		markHurts:  vector.AlertWarning,
		markNote:   vector.ActionInfoOutline,
	}
	out := make(map[mark]*widget.Icon, len(data))
	for kind, vg := range data {
		icon, err := widget.NewIcon(vg)
		if err != nil {
			panic("a mark icon does not decode: " + err.Error())
		}
		out[kind] = icon
	}
	return out
}()

// markSide is an icon's size: the subtitle's own line height.
const markSide = unit.Dp(15)

// drawMark is one mark's icon.
func drawMark(gtx layout.Context, th *material.Theme, kind mark) layout.Dimensions {
	ink := colorMuted
	if kind == markHurts {
		ink = colorWarn
	}
	side := gtx.Dp(markSide)
	gtx.Constraints = layout.Exact(image.Pt(side, side))
	return markIcons[kind].Layout(gtx, ink)
}

// tileMarks is what a tile shows after its subtitle: its marks, and the
// notes its note mark shows under the pointer. id names the tile, for the
// pointer's events.
type tileMarks struct {
	set   mark
	notes []string
	id    noteTag
}

// noteTag is a tile's note mark as a pointer target: its depth and its key.
type noteTag struct{ at, key string }

// layoutMarks is a tile's marks in a row, a small gap between them and after
// the words when they follow some, and the notes in a label under the note
// mark while the pointer rests on it.
func (s *State) layoutMarks(gtx layout.Context, th *material.Theme, marks tileMarks, afterWords bool) layout.Dimensions {
	var children []layout.FlexChild
	for _, kind := range markOrder {
		if marks.set&kind == 0 {
			continue
		}
		if afterWords || len(children) > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout))
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if kind != markNote || len(marks.notes) == 0 {
					return drawMark(gtx, th, kind)
				}
				return s.noteMark(gtx, th, marks)
			}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

// noteMark is a note's icon, which shows its words while the pointer is on it.
// Only Enter and Leave move the note: the Cancel every new target is sent
// first says nothing about the pointer, and a target gone from the frame
// gets a Leave from the router's next hit test.
func (s *State) noteMark(gtx layout.Context, th *material.Theme, marks tileMarks) layout.Dimensions {
	for {
		got, ok := gtx.Event(pointer.Filter{Target: marks.id, Kinds: pointer.Enter | pointer.Leave})
		if !ok {
			break
		}
		switch got, _ := got.(pointer.Event); {
		case got.Kind == pointer.Enter:
			s.noteOver = marks.id
		case got.Kind == pointer.Leave && s.noteOver == marks.id:
			s.noteOver = noteTag{}
		}
	}
	dims := drawMark(gtx, th, markNote)
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	event.Op(gtx.Ops, marks.id)
	area.Pop()
	if s.noteOver == marks.id {
		hoverLabel(gtx, th, strings.Join(marks.notes, "\n"), dims, true)
	}
	return dims
}

// markLegend is the legend's lines, one a mark.
var markLegend = []helpLine{
	{mark: markStatue, lead: "Statue", words: "Stands where it is summoned and never moves."},
	{mark: markExtra, lead: "Extra", words: "Something the game never places: cut or unused content, or an odd look found in its files."},
	{mark: markOnFoot, lead: "On foot", words: "Rides a horse or another mount in the game. A summon cannot be seated on one, so it arrives on foot."},
	{mark: markHurts, lead: "Hurts you", words: "Can hurt you, the player who summons it."},
	{mark: markNote, lead: "Note", words: "Works, but differs from the game: point at it to read how. On a card, one of the creatures inside has one."},
}

// marksAndNotes reads a variant's marks and the notes a player is shown,
// from research's record: its tags (statue, extra), its warning, and its
// model's hazards. The warning is research's own words, kept for the record,
// so each kind of clause is said here as a player would want it said.
//
// Only what is true of this row reaches it. A clause research writes for the
// row - "stands where summoned", "rides a mount in the world" - is the row's,
// and so is a tag; a family hazard is the model's, and most are true of some
// of its rows only (only the Foot Soldiers' crew stand, only the Silver
// Tear's shape-changer vanishes), so only those true of every row are shown
// (hazardWords).
func marksAndNotes(leaf application.HierarchyEntry) (mark, []string) {
	var marks mark
	for _, tag := range strings.Split(leaf.Tags["tag"], "|") {
		switch tag {
		case "statue":
			marks |= markStatue
		case "extra":
			marks |= markExtra
		}
	}
	if _, hurts := application.HurtsSummoner(leaf.Model); hurts && leaf.Trunk == "creature" {
		marks |= markHurts
	}
	var notes []string
	note := func(words string) {
		if words != "" && !slices.Contains(notes, words) {
			notes = append(notes, words)
		}
	}
	all := clauses(leaf.Warning)
	paired := slices.ContainsFunc(all, func(clause string) bool { return strings.HasPrefix(clause, "a pair: ") })
	for _, clause := range all {
		head, rest, _ := strings.Cut(clause, ": ")
		switch {
		case clause == "", clause == "it fights", strings.HasPrefix(clause, "extra"),
			strings.HasPrefix(clause, "unused content"):
			// What the extra mark already says.
		case researchOnly.MatchString(plain(clause)):
			// A row, an animation or a witness's word: research's, not the
			// player's.
		case strings.HasPrefix(clause, "stands where summoned"), strings.HasPrefix(clause, "it stands where summoned"),
			strings.HasPrefix(clause, "a machine"):
			marks |= markStatue
		case head == "does not move or fight":
			// What follows is why, or what it does instead (the Basilisk
			// Eyes' deathblight); the first the statue mark says.
			marks |= markStatue
			if !strings.Contains(rest, "stands where summoned") {
				note(sentence(rest))
			}
		case strings.HasPrefix(clause, "does not fight"):
			// A statue says it already; one that moves - the Chariot - says it
			// here (doesNotFight below).
		case strings.HasPrefix(clause, "rides a mount in the world"):
			marks |= markOnFoot
		case strings.HasPrefix(clause, "rides a mount in some placements"):
			note("Rides a mount in some of its placements in the game; summoned, it arrives on foot.")
		case clause == "vanishes":
			// A pair's note says what becomes of it.
			if !paired {
				note("Vanishes soon after it arrives.")
			}
		case head == "family hazard":
			for _, code := range strings.Split(rest, "|") {
				if code == "hurts" {
					marks |= markHurts
				}
				note(hazardWords[code])
			}
		case strings.HasPrefix(clause, "drives at what it faces"):
			marks |= markHurts
			note("Drives at whatever it faces and runs it over, you included.")
		case strings.HasPrefix(head, "not every placement's"):
			note("Has " + plain(rest) + ".")
		case strings.HasPrefix(head, "not the game's fight as the product delivers"):
			// The world's own placement mark, which a summon carries.
			note("Carries " + strings.TrimSuffix(plain(rest), " added") + ".")
		case strings.HasPrefix(head, "not the game's fight"):
			note("Unlike the game: " + plain(rest) + ".")
		case head == "armed by the editor":
			note("Armed by the editor, with gear the game never gives it.")
		case head == "a pair":
			note(sentence(rest))
		case strings.HasPrefix(clause, "the ") && strings.Contains(clause, "placements"):
			note("Has " + plain(clause) + ".")
		default:
			note(sentence(clause))
		}
	}
	if marks&markStatue == 0 && slices.ContainsFunc(all, doesNotFight) {
		notes = append([]string{"Does not fight."}, notes...)
	}
	if len(notes) > 0 {
		marks |= markNote
	}
	return marks, notes
}

func doesNotFight(clause string) bool { return strings.HasPrefix(clause, "does not fight") }

// hazardWords are research's family hazards (compatibility.csv) true of every
// row of the model, as a player reads them. The others are shown only where
// research says them of a row: rider and stands (a clause or a tag), vanishes
// (a clause), and spawner, partner, team-change, phased and frozen-pose,
// which concern some rows or none that are offered.
var hazardWords = map[string]string{
	"scale":           "A large body: summon it in the open.",
	"mount":           "A mount: it arrives without its rider.",
	"warp":            "Keeps away from what it fights, so the summon's pull back to you takes most of its fight.",
	"anchored":        "Turns toward its enemy, but never moves and deals no damage.",
	"plate":           "Shows a boss name and health bar when it is hit.",
	"invisible":       "Its body is not drawn: only its effects and weapon show.",
	"needs-character": "Fights characters only, not ordinary enemies.",
	"retaliates":      "Fights only once it is hit.",
	"flees":           "Runs away from enemies.",
}

// researchAside is a parenthesis research writes for itself: a row, a goal,
// a file, a witness's word or a question still open. researchOnly is a
// clause that still names a row or an animation, or the author, without one.
var (
	researchAside = regexp.MustCompile(` ?\((?:open|[^)]*(?:\d{3,}|\.csv|witness|goal|#)[^)]*)\)`)
	researchOnly  = regexp.MustCompile(`\d{4,}|witness`)
)

// clauses are a warning's clauses, split at "; " outside parentheses.
func clauses(warning string) []string {
	var out []string
	depth, start := 0, 0
	for at := 0; at < len(warning); at++ {
		switch {
		case warning[at] == '(':
			depth++
		case warning[at] == ')':
			depth = max(depth-1, 0)
		case depth == 0 && strings.HasPrefix(warning[at:], "; "):
			out = append(out, warning[start:at])
			start = at + 2
		}
	}
	return append(out, warning[start:])
}

// plain is a clause without research's asides.
func plain(words string) string {
	return strings.TrimSpace(researchAside.ReplaceAllString(words, ""))
}

// sentence is a clause as a sentence: its asides gone, capitalised, closed.
func sentence(words string) string {
	words = strings.TrimPrefix(plain(words), "and ")
	words = strings.Replace(words, "summoned it is ", "summoned, it is ", 1)
	if words == "" {
		return ""
	}
	words = strings.ToUpper(words[:1]) + words[1:]
	if !strings.HasSuffix(words, ".") {
		words += "."
	}
	return words
}
