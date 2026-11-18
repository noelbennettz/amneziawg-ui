// Package serverlist is the body of the page below the create form: one card
// per server, or a hint while there are none.
package serverlist

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/env"
	"amneziawg-web-ui/web-ui/internal/ui/serverlist/serverentry"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// List owns the cards on screen, keyed by server ID so a traffic snapshot can
// find the card it belongs to.
type List struct {
	env *env.Env

	box *fyne.Container
	// emptyCard holds the hint, and is hidden as a whole: hiding the label
	// alone would leave the card's surface drawn around nothing - a layout
	// measures a stack's invisible children too, and a card is a stack.
	emptyCard fyne.CanvasObject
	cards     map[string]*serverentry.Card
	narrow    bool

	object fyne.CanvasObject
}

// New builds an empty list; Render fills it.
func New(e *env.Env) *List {
	l := &List{env: e, box: container.NewVBox(), cards: map[string]*serverentry.Card{}}

	hint := widgets.MutedNote(lang.L("No servers created yet. Create your first server above."))
	l.emptyCard = widgets.Card(container.NewCenter(hint))

	l.object = container.NewVBox(l.box, l.emptyCard)
	return l
}

// CanvasObject is the list as the page places it.
func (l *List) CanvasObject() fyne.CanvasObject {
	return l.object
}

// SetNarrow restacks the cards for a compact page: a row's actions drop under
// its name, and the peer rows fold with them.
func (l *List) SetNarrow(narrow bool) {
	l.narrow = narrow
	for _, card := range l.cards {
		card.SetNarrow(narrow)
	}
	l.box.Refresh()
}

// Render rebuilds every card from a fresh server list. Must run on the UI
// goroutine.
func (l *List) Render(servers []api.Server) {
	l.cards = map[string]*serverentry.Card{}
	l.box.Objects = nil

	for _, srv := range servers {
		card := serverentry.New(l.env, srv)
		card.SetNarrow(l.narrow)
		l.cards[srv.ID] = card
		l.box.Add(card.CanvasObject())
	}

	if len(servers) > 0 {
		l.emptyCard.Hide()
	} else {
		l.emptyCard.Show()
	}
	l.box.Refresh()
}

// ApplyTraffic pushes counters into the existing labels, so the page updates
// without rebuilding (and without losing scroll position or focus). Must run
// on the UI goroutine.
func (l *List) ApplyTraffic(iface map[string]api.InterfaceTraffic, peers map[string]map[string]api.ClientTraffic) bool {
	changed := false
	for id, card := range l.cards {
		traffic, ok := iface[id]
		if card.ApplyInterfaceTraffic(traffic, ok) {
			changed = true
		}
		if card.ApplyPeerTraffic(peers[id]) {
			changed = true
		}
	}
	return changed
}
