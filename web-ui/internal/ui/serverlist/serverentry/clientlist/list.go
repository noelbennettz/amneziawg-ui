// Package clientlist is the block of peer rows at the bottom of a server
// card, with the heading that counts them.
package clientlist

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/env"
	"amneziawg-web-ui/web-ui/internal/ui/serverlist/serverentry/clientlist/cliententry"
	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// List is the peers of one server, keyed by client ID so a traffic snapshot
// can find its row.
type List struct {
	rows   map[string]*cliententry.Row
	object fyne.CanvasObject
}

// New renders the peers of server.
func New(e *env.Env, server api.Server) *List {
	l := &List{rows: map[string]*cliententry.Row{}}

	clients := server.Clients
	if len(clients) == 0 {
		l.object = container.NewPadded(widgets.SmallText(lang.L("No clients yet."), style.Muted))
		return l
	}

	box := container.NewVBox()
	for _, client := range clients {
		row := cliententry.New(e, server, client)
		l.rows[client.ID] = row
		box.Add(row.CanvasObject())
	}
	l.object = box
	return l
}

// CanvasObject is the block as the card places it.
func (l *List) CanvasObject() fyne.CanvasObject {
	return l.object
}

// SetNarrow restacks every peer row for a compact page.
func (l *List) SetNarrow(narrow bool) {
	for _, row := range l.rows {
		row.SetNarrow(narrow)
	}
}

// ApplyTraffic pushes a per-client snapshot into the rows. A peer the
// snapshot does not mention has simply never connected. Must run on the UI
// goroutine.
func (l *List) ApplyTraffic(traffic map[string]api.ClientTraffic) bool {
	changed := false
	for id, row := range l.rows {
		data, ok := traffic[id]
		if !ok {
			data = api.ClientTraffic{Received: "0 B", Sent: "0 B", LastHandshake: lang.L("Never")}
		}
		if row.Apply(data) {
			changed = true
		}
	}
	return changed
}
