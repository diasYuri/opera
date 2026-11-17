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

func (o *overlayModel) showSelector(selector list.Model) {
	o.selector = selector
	o.selecting = true
	o.modelMenu = false
	o.panelOpen = false
}

func (o *overlayModel) dismissSelector() {
	o.selecting = false
	o.modelMenu = false
	o.selectionKind = ""
}

func (o *overlayModel) showPanel(title string, panel viewport.Model) {
	o.panelTitle = title
	o.panel = panel
	o.panelOpen = true
	o.selecting = false
}

func (o *overlayModel) dismissPanel() { o.panelOpen = false }
