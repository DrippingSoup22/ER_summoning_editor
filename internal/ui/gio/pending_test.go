package gio

// The pending dialog: every staged change, grouped by where it was made, and
// the Drop behind each one and behind each group.

import (
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// allEntries is every change the dialog lists, section by section.
func allEntries(sections []pendingSection) []pendingEntry {
	var entries []pendingEntry
	for _, section := range sections {
		for _, group := range section.Groups {
			entries = append(entries, group.Entries...)
		}
	}
	return entries
}

// groupNamed is the group of that name in the dialog, or nil.
func groupNamed(sections []pendingSection, name string) *pendingGroup {
	for _, section := range sections {
		for index := range section.Groups {
			if section.Groups[index].Name == name {
				return &section.Groups[index]
			}
		}
	}
	return nil
}

// twoAshes picks the Ash a test edits and a second one to leave alone, which is
// what makes "drops exactly its own" checkable.
func twoAshes(t *testing.T, state *State) (host *ashView, other *ashView) {
	t.Helper()
	index, host := editable(t, state)
	state.selected = index
	for at := range state.LoadedSave().Ashes {
		if at != index {
			return host, &state.LoadedSave().Ashes[at]
		}
	}
	t.Fatal("the fixture holds only one Ash")
	return nil, nil
}

// Every kind of staged change appears once, under the Ash it was made on, and
// each carries a Drop. The list is built from the session rather than from
// frame state, so this is also the check that the two never drift apart.
func TestThePendingListHoldsEveryKindOfChange(t *testing.T) {
	state := loadedState(t)
	host, _ := twoAshes(t, state)
	payload, _ := aSpirit(t, state)

	// A formation on one Ash and a price on another - two different shapes of
	// change at once. Conversions were the third and this editor no longer
	// makes them; the command line still does.
	state.drop(host, maxSlots-1, state.slotsFor(host), payload)
	priced := pricedAsh(t, state)
	typePrice(state, priced, "42", "0")

	sections := state.pendingSections()
	entries := allEntries(sections)
	if len(entries) != state.PendingCount() {
		t.Fatalf("the list holds %d entries and the count says %d", len(entries), state.PendingCount())
	}
	for _, entry := range entries {
		if entry.Label == "" || entry.Detail == "" || entry.Drop == nil {
			t.Errorf("an entry says %q / %q, or has no Drop", entry.Label, entry.Detail)
		}
	}
	// The formation sits under its Ash and says what it summons; the price
	// under its own, saying what it will cost.
	said := map[string]string{}
	for _, name := range []string{host.Name, priced.Name} {
		if group := groupNamed(sections, name); group != nil {
			for _, entry := range group.Entries {
				said[entry.Label] = entry.Detail
			}
		}
	}
	if !strings.Contains(said["Formation"], "summon") {
		t.Errorf("the formation under %s says %q, not what it summons", host.Name, said["Formation"])
	}
	if !strings.Contains(said["Price"], "42 FP") {
		t.Errorf("the price under %s says %q, not what it will cost", priced.Name, said["Price"])
	}
}

// A character's changes are one group, in slot order, each Ash a line that
// says what happens to it: the given first, then the levelled, then the taken,
// whatever order the Ashes come in.
func TestACharactersAshesAreOneGroup(t *testing.T) {
	state := loadedState(t)
	grants := []*application.StagedGrant{
		{Slot: 0, Character: "Tarnished", GoodsFamily: 1, Name: "Lone Wolf Ashes", Was: 4, Take: true},
		{Slot: 0, Character: "Tarnished", GoodsFamily: 2, Name: "Mimic Tear Ashes", Level: 10, Was: 2},
		{Slot: 0, Character: "Tarnished", GoodsFamily: 3, Name: "Nomad Ashes", Level: 10, Was: -1},
		{Slot: 3, Character: "Second", GoodsFamily: 1, Name: "Lone Wolf Ashes", Level: 10, Was: -1},
	}
	groups := state.characterGroups(grants)
	if len(groups) != 2 || groups[0].Name != "Tarnished" || groups[1].Name != "Second" || len(groups[0].Entries) != 3 {
		t.Fatalf("four grants for two characters made %+v", groups)
	}
	for _, want := range []struct {
		entry pendingEntry
		words string
	}{
		{groups[0].Entries[0], "Given at +10"},
		{groups[0].Entries[1], "From +2 to +10"},
		{groups[0].Entries[2], "Taken away"},
		{groups[1].Entries[0], "Given at +10"},
	} {
		if want.entry.Detail != want.words || want.entry.Ash != want.entry.Label {
			t.Errorf("%s says %q with icon %q, want %q with its own", want.entry.Label, want.entry.Detail,
				want.entry.Ash, want.words)
		}
	}
}

// An Ash's formation and its price are one group, and its Drop all drops both
// and nothing else; a group of one change has no Drop all, its change's own
// Drop being the same button.
func TestAnAshsChangesAreOneGroupWithADropAll(t *testing.T) {
	state := loadedState(t)
	var both, other *ashView
	for index := range state.LoadedSave().Ashes {
		ash := &state.LoadedSave().Ashes[index]
		switch {
		case ash.locked() != "":
		case both == nil && !ash.PriceVaries:
			both = ash
		case other == nil:
			other = ash
		}
	}
	if both == nil || other == nil {
		t.Skip("the fixture holds no Ash to compose and price, beside another")
	}
	payload, _ := aSpirit(t, state)
	state.drop(both, maxSlots-1, state.slotsFor(both), payload)
	typePrice(state, both, "42", "0")
	state.drop(other, maxSlots-1, state.slotsFor(other), payload)

	sections := state.pendingSections()
	if group := groupNamed(sections, both.Name); group == nil || len(group.Entries) != 2 {
		t.Fatalf("%s's formation and price are not one group of two: %+v", both.Name, group)
	}
	var dropAll func()
	for _, line := range pendingLines(sections, nil) {
		if line.group != nil && line.group.Name == other.Name && line.drop != nil {
			t.Errorf("%s, one change, has a Drop all", other.Name)
		}
		if line.group != nil && line.group.Name == both.Name {
			dropAll = line.drop
		}
	}
	if dropAll == nil {
		t.Fatalf("%s's two changes have no Drop all", both.Name)
	}
	dropAll()
	if state.PendingCount() != 1 || state.edits[both.GoodsFamily] != nil || state.edits[other.GoodsFamily] == nil {
		t.Errorf("Drop all left %d pending, want only %s's formation", state.PendingCount(), other.Name)
	}
}

// The ✕ drops one change and leaves the others, and drops the frame state
// drawing it at the same time - the panel and the session are two copies of
// one fact and must not survive each other.
func TestDroppingOnePendingChangeLeavesTheRest(t *testing.T) {
	state := loadedState(t)
	host, _ := twoAshes(t, state)
	payload, _ := aSpirit(t, state)
	state.drop(host, maxSlots-1, state.slotsFor(host), payload)
	priced := pricedAsh(t, state)
	typePrice(state, priced, "42", "0")
	before := state.PendingCount()
	if before != 2 {
		t.Fatalf("%d changes are staged, want 2", before)
	}

	// Matched on what the entry IS: the fixture can hand the same Ash back for
	// the price, and then both are in one group.
	for _, entry := range allEntries(state.pendingSections()) {
		if entry.Label == "Formation" {
			entry.Drop()
		}
	}

	if state.PendingCount() != 1 {
		t.Errorf("%d changes are pending after dropping one of two", state.PendingCount())
	}
	if state.edits[host.GoodsFamily] != nil {
		t.Error("the composed formation survived the ✕ that dropped it")
	}
	if got := state.slotsFor(host); got != host.Slots {
		t.Error("the panel still draws a formation the session no longer holds")
	}
	if state.PendingCount() != 1 {
		t.Error("dropping the formation also dropped the unrelated price")
	}
}

// A folded section or group hides what it holds and keeps a way to drop it: its
// count, and a Drop all - or, over one change, that change's Drop, whose own
// line is hidden. Open over one change it has no button of its own.
func TestAFoldHidesWhatItHoldsAndKeepsItsDrop(t *testing.T) {
	var dropped []string
	change := func(label string) pendingEntry {
		return pendingEntry{Label: label, Detail: label, Drop: func() { dropped = append(dropped, label) }}
	}
	sections := []pendingSection{
		{Name: "Characters", Groups: []pendingGroup{
			{Key: "character:0", Name: "Tarnished", Entries: []pendingEntry{change("given"), change("taken")}},
		}},
		{Name: "Editor", Groups: []pendingGroup{
			{Key: "ash:1", Name: "Lone Wolf Ashes", Entries: []pendingEntry{change("price")}},
		}},
	}
	head := func(lines []pendingLine, key string) pendingLine {
		for _, line := range lines {
			if line.key == key {
				return line
			}
		}
		t.Fatalf("no line folds under %s", key)
		return pendingLine{}
	}
	changes := func(lines []pendingLine) (n int) {
		for _, line := range lines {
			if line.entry != nil {
				n++
			}
		}
		return n
	}

	open := pendingLines(sections, nil)
	if changes(open) != 3 || head(open, "ash:1").drop != nil {
		t.Errorf("open, the list shows %d changes, and a group of one has a Drop of its own: %t",
			changes(open), head(open, "ash:1").drop != nil)
	}
	groups := pendingLines(sections, map[string]bool{"character:0": true, "ash:1": true})
	tarnished, wolf := head(groups, "character:0"), head(groups, "ash:1")
	if changes(groups) != 0 || !tarnished.folded || tarnished.count != 2 || wolf.drop == nil {
		t.Fatalf("folded, the groups show %d changes, Tarnished folded %t over %d, the wolf's Drop %t",
			changes(groups), tarnished.folded, tarnished.count, wolf.drop != nil)
	}
	tarnished.drop()
	section := head(pendingLines(sections, map[string]bool{"Characters": true}), "Characters")
	if section.count != 2 || section.drop == nil {
		t.Fatalf("a folded section holds %d changes, Drop all %t", section.count, section.drop != nil)
	}
	section.drop()
	if strings.Join(dropped, ",") != "given,taken,given,taken" {
		t.Errorf("the group's and then the section's Drop all dropped %v", dropped)
	}
	if lines := pendingLines(sections, map[string]bool{"Characters": true}); changes(lines) != 1 || len(lines) != 4 {
		t.Errorf("a folded section leaves %d lines and %d changes, want its title, the Editor's two and the price",
			len(lines), changes(lines))
	}
}

// A Drop redraws the dialog in the frame that reads it, as a fold does: a
// frame without the dialog shows the window undimmed, a flash over the whole
// screen at every click.
func TestThePendingDialogStaysDrawnThroughADrop(t *testing.T) {
	state := loadedState(t)
	host, _ := twoAshes(t, state)
	payload, _ := aSpirit(t, state)
	state.drop(host, maxSlots-1, state.slotsFor(host), payload)
	typePrice(state, host, "42", "0")
	if state.PendingCount() != 2 {
		t.Fatalf("staging a formation and a price left %d changes, want 2", state.PendingCount())
	}
	h := newHands(t, state) // its first frame takes the load in, closing any dialog
	state.pendingOpen = true
	h.frame()
	h.click("Drop")
	if state.PendingCount() != 1 {
		t.Fatalf("a Drop left %d changes, want 1", state.PendingCount())
	}
	if _, drawn := h.find("Drop"); !drawn {
		t.Error("the frame that read the Drop drew no dialog")
	}
}
