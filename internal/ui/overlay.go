package ui

import (
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
)

// overlayModel owns transient selection and detail-panel interaction state.
// The chat shell routes keys and asynchronous results to this active surface.
type overlayModel struct {
	selector      list.Model
	panel         viewport.Model
	panelTitle    string
	selectionKind string
	modelMenu     bool
	selecting     bool
	panelOpen     bool
}
