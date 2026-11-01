package gio

// Opening a save.
//
// The picker and the read both run off the frame goroutine: a native dialog is
// modal to the operating system rather than to this application, and reading a
// 26 MB save decodes every catalogued goods row and every shipped formation
// block. Neither may happen inside a frame.
//
// The loaded path is never written to, then or later. That is a property of
// the application layer rather than of this file, but it is the reason the
// picker asks again at save time instead of remembering an answer.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/platform/dialogs"
)

// StartOpenSave asks for a save and loads it. A second call while a load is in
// flight is ignored rather than queued: the player pressed a button twice.
func (s *State) StartOpenSave() {
	if !s.begin("Choosing a save…") {
		return
	}
	go s.openWorker()
}

// StartLoadSave loads a known path without asking. Used by the development
// environment override and by tests, which have no dialog to answer.
func (s *State) StartLoadSave(path string) {
	if !s.begin("Reading the save…") {
		return
	}
	go s.loadWorker(path)
}

func (s *State) openWorker() {
	path, err := s.dialogs.OpenSave()
	if err != nil {
		if errors.Is(err, dialogs.ErrCanceled) {
			// Nothing happened, so nothing is reported. A cancel that leaves a
			// message behind reads as a failure the player has to interpret.
			s.finish(nil, notice{})
			return
		}
		s.finish(nil, notice{text: "Could not open the file picker: " + err.Error(), bad: true})
		return
	}
	s.progress("Reading the save…")
	s.loadWorker(path)
}

// loadWorker holds the session for the whole read. On failure the session
// keeps whatever save it already had, so the player is still editing the file
// they were editing.
func (s *State) loadWorker(path string) {
	if err := s.session.Load(context.Background(), path); err != nil {
		s.finish(nil, notice{text: describeLoadError(path, err), bad: true})
		return
	}
	view, err := s.readView()
	if err != nil {
		s.finish(nil, notice{text: describeLoadError(path, err), bad: true})
		return
	}
	s.finish(view, notice{text: fmt.Sprintf("Opened %s.", view.Name)})
}

// readView builds the immutable snapshot the frame renders. It runs in the
// worker, while the worker still holds the session, which is what lets the
// frame describe the save without ever calling into it.
func (s *State) readView() (*savedView, error) {
	save := s.session.Save()
	ashes, err := s.session.AshStates()
	if err != nil {
		return nil, err
	}
	hierarchy, err := s.session.Hierarchy()
	if err != nil {
		return nil, err
	}
	labels, err := s.session.ActorLabels()
	if err != nil {
		return nil, err
	}
	bodies, err := s.session.Bodies()
	if err != nil {
		return nil, err
	}
	// The ten character slots. Read here, in the worker, because it costs a
	// 26 MB read of the file and nothing about it changes until a commit.
	people, err := s.session.Characters()
	if err != nil {
		return nil, err
	}
	rail := buildAshes(ashes, labels)
	art, err := newPictures(hierarchy)
	if err != nil {
		return nil, err
	}
	spirits, err := buildFamilies(hierarchy, art)
	if err != nil {
		return nil, err
	}
	// The custom characters' card, which the session describes apart from the
	// hierarchy because staging a build changes it (builder.go).
	custom, err := customFamily(s.session.CustomCharacterCard(), art)
	if err != nil {
		return nil, err
	}
	spirits = withCustom(spirits, custom)
	view := &savedView{
		Name:              filepath.Base(save.Path),
		Path:              save.Path,
		RegulationVersion: save.RegulationVersion,
		Ashes:             rail,
		labels:            labels,
		bodies:            bodies,
		all:               spirits,
		pictures:          art,
		// Pruned for nothing chosen: the switch and the subcategory are the
		// frame's, which re-prunes on its first draw if they differ.
		Families:      prune(spirits, pruning{}),
		Categories:    categoryNames(hierarchy),
		Subcategories: subcategoryNames(spirits),
		Characters:    buildCharacters(people),
	}
	for index, ash := range rail {
		if !application.FixedHost(ash.GoodsFamily) {
			view.Containers = append(view.Containers, index)
		}
	}
	view.CategoryLabels = labelsOf(view.Categories)
	view.SubcategoryLabels = make(map[string][]string, len(view.Subcategories))
	for category, words := range view.Subcategories {
		view.SubcategoryLabels[category] = labelsOf(words)
	}
	for _, family := range view.Families {
		view.Spirits += family.spirits()
	}
	view.nameHeld()
	return view, nil
}

// describeLoadError turns a typed failure into a sentence a player can act on.
//
// Every case here is reached with errors.Is or errors.As and never by matching
// message text, which is written for people and changes freely. The default is
// the error itself: a failure this function has not been taught is still worth
// showing, and inventing a friendlier wording for an unknown problem hides it.
func describeLoadError(path string, err error) string {
	name := filepath.Base(path)
	var unsupported *application.UnsupportedRegulationError
	switch {
	case errors.As(err, &unsupported):
		return fmt.Sprintf(
			"%s carries regulation %d; this editor writes %s only.",
			name, unsupported.Found, application.SupportedRegulations())
	case errors.Is(err, application.ErrNotDecryptedPS4Save):
		return fmt.Sprintf("%s is not a decrypted PS4 save; PC saves are not supported.", name)
	case errors.Is(err, application.ErrSourceMismatch):
		return fmt.Sprintf("%s changed while being read: open it again.", name)
	default:
		return fmt.Sprintf("Could not open %s: %v", name, err)
	}
}
