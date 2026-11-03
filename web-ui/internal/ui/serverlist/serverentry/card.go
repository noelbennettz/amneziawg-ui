// Package serverentry is one server's card: its name, addresses and counters,
// the start/stop control, the actions behind the overflow menu, and its
// clients.
package serverentry

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/dialogs"
	"amneziawg-web-ui/web-ui/internal/ui/env"
	"amneziawg-web-ui/web-ui/internal/ui/serverlist/serverentry/clientlist"
	"amneziawg-web-ui/web-ui/internal/ui/serverlist/serverentry/clientlist/cliententry/clientdialogs"
	"amneziawg-web-ui/web-ui/internal/ui/serverlist/serverentry/serverdialogs"
	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/testhook"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// Card is one server's card. It keeps the labels the traffic feed updates in
// place, and the client rows below them.
type Card struct {
	env    *env.Env
	server api.Server

	rx     *canvas.Text
	tx     *canvas.Text
	uptime *canvas.Text

	head    *widgets.PairBox
	actions *widgets.SplitRowBox
	clients *clientlist.List

	object fyne.CanvasObject
}

// New renders the card for server as the list last fetched it.
func New(e *env.Env, srv api.Server) *Card {
	c := &Card{env: e, server: srv}

	title := widgets.CardTitle(srv.Name)

	meta := widgets.MetaLabel(serverMeta(srv))

	// The avatar is the tonal disc M3 puts at the head of a list row.
	avatar := container.NewStack(
		roundedRect(style.SurfaceHigh, 18),
		container.NewCenter(widgets.Icon(theme.ComputerIcon(), 20)),
	)

	c.rx = widgets.SmallText("—", style.Muted)
	c.tx = widgets.SmallText("—", style.Muted)
	c.uptime = widgets.SmallText("", style.Muted)
	throughput := container.NewHBox(
		container.NewCenter(widgets.SmallIcon(theme.DownloadIcon())), c.rx,
		container.NewCenter(widgets.SmallIcon(theme.UploadIcon())), c.tx,
		c.uptime,
	)

	status := widgets.StatusChip(stringsUpper(lang.L(srv.Status)), statusColor(srv.Status))

	startStop := widgets.IconButton(theme.MediaPlayIcon(), func() { c.setRunning(true) })
	if srv.Status == "running" {
		startStop.SetIcon(theme.MediaStopIcon())
		startStop.OnTapped = func() { c.setRunning(false) }
	}

	menu := widgets.MenuButton("server."+srv.ID+".menu", []widgets.MenuItem{
		{ID: "config", Label: lang.L("Show config"), Do: func() { serverdialogs.ShowConfig(e, srv.ID) }},
		{ID: "addClient", Label: lang.L("Add client"), Do: func() { clientdialogs.ShowEditor(e, srv, nil) }},
		{ID: "toggle", Label: serverToggleLabel(srv), Do: func() { c.setRunning(srv.Status != "running") }},
		{ID: "delete", Label: lang.L("Delete server…"), Do: c.confirmDelete},
	})

	// The header is a row: the server on the left, what it is doing and the
	// actions on the right. On a phone it folds twice - the whole right-hand
	// group drops under the name (widgets.Pair), and inside it the traffic
	// counters take a line of their own under the state (widgets.SplitRow).
	c.actions = widgets.NewSplitRow(style.Space3,
		container.NewCenter(status),
		container.NewCenter(throughput),
		container.NewHBox(container.NewCenter(startStop), container.NewCenter(menu)),
	)

	// A border rather than a row: the avatar takes its 36 points and the name
	// and meta get everything left over. An HBox would give the text block its
	// minimum width instead, which is the width of its longest word - the meta
	// then wraps into four lines in the middle of a 1300-point card.
	c.head = widgets.NewPair(style.Space3, 44,
		container.NewBorder(nil, nil,
			container.NewHBox(
				container.NewCenter(container.NewGridWrap(fyne.NewSize(36, 36), avatar)),
				widgets.Gap(style.Space2),
			),
			nil,
			container.NewVBox(title, meta),
		),
		c.actions,
	)

	addClient := addClientButton(e, srv)
	clientsHead := container.NewBorder(nil, nil,
		container.NewCenter(widgets.SmallText(lang.L("Clients ({{.Count}})", map[string]any{"Count": len(srv.Clients)}), style.Muted)),
		container.NewCenter(addClient),
	)

	testhook.Register("server."+srv.ID+".menu", menu)
	testhook.Register("server."+srv.ID+".toggle", startStop)
	testhook.Register("server."+srv.ID+".addClient", addClient)

	c.clients = clientlist.New(e, srv)

	content := container.NewVBox(
		c.head,
		widgets.Separator(),
		clientsHead,
		c.clients.CanvasObject(),
	)

	c.object = widgets.Card(content)
	return c
}

// CanvasObject is the card as the list places it.
func (c *Card) CanvasObject() fyne.CanvasObject { return c.object }

// SetNarrow restacks the header and the peer rows for a compact page.
func (c *Card) SetNarrow(narrow bool) {
	c.head.SetNarrow(narrow)
	c.actions.SetNarrow(narrow)
	c.clients.SetNarrow(narrow)
	c.object.Refresh()
}

// ApplyInterfaceTraffic updates the RX/TX counters and the uptime of the
// interface; ok is false for a server whose interface is down. Must run on the
// UI goroutine.
func (c *Card) ApplyInterfaceTraffic(traffic api.InterfaceTraffic, ok bool) bool {
	rx, tx, uptime := "—", "—", 0.0
	if ok {
		rx, tx, uptime = traffic.RX, traffic.TX, traffic.UptimeSeconds
	}
	when := uptimeText(uptime)
	changed := c.rx.Text != rx || c.tx.Text != tx || c.uptime.Text != when

	c.rx.Text = rx
	c.rx.Refresh()
	c.tx.Text = tx
	c.tx.Refresh()
	c.uptime.Text = when
	c.uptime.Refresh()

	return changed
}

// uptimeText is the trailing uptime of the counter line; empty while the
// uptime is zero, which is a down interface or one not polled yet.
func uptimeText(uptime float64) string {
	if uptime <= 0 {
		return ""
	}
	return "·  " + lang.L("up {{.Uptime}}", map[string]any{"Uptime": widgets.Uptime(uptime)})
}

// ApplyPeerTraffic hands the per-client snapshot down to the rows. Must run
// on the UI goroutine.
func (c *Card) ApplyPeerTraffic(traffic map[string]api.ClientTraffic) bool {
	return c.clients.ApplyTraffic(traffic)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// serverMeta is the one line under the server's name: the facts an operator
// scans for, addresses in monospace.
func serverMeta(srv api.Server) string {
	meta := fmt.Sprintf("Port %d · %s", srv.Port, srv.Subnet)
	if srv.SubnetV6 != "" {
		meta += " · " + srv.SubnetV6
	}
	return meta + fmt.Sprintf(" · MTU %d", srv.MTU)
}

func addClientButton(e *env.Env, srv api.Server) *widgets.Button {
	add := widgets.NewButton(lang.L("Add client"), theme.ContentAddIcon(), func() {
		clientdialogs.ShowEditor(e, srv, nil)
	})
	add.Importance = widget.MediumImportance
	return add
}

func serverToggleLabel(srv api.Server) string {
	if srv.Status == "running" {
		return lang.L("Stop server")
	}
	return lang.L("Start server")
}

func statusColor(status string) color.NRGBA {
	if status == "running" {
		return style.Success
	}
	return style.Muted
}

// roundedRect is the tonal disc behind an avatar glyph.
func roundedRect(fill color.NRGBA, radius float32) fyne.CanvasObject {
	rect := canvas.NewRectangle(fill)
	rect.CornerRadius = radius
	return rect
}

func stringsUpper(s string) string {
	// The badge is set in the same case the translation gives it: upper case
	// would shout in some languages and not in others, so this only trims.
	return s
}

// ── Actions ──────────────────────────────────────────────────────────────────

func (c *Card) setRunning(start bool) {
	e, srv := c.env, c.server

	go func() {
		var err error
		if start {
			err = e.Backend.StartServer(srv.ID)
		} else {
			err = e.Backend.StopServer(srv.ID)
		}
		if err != nil {
			e.Notify.Fail(err)
			return
		}
		name := map[string]any{"Name": srv.Name}
		if start {
			e.Notify.OK(lang.L("Server \"{{.Name}}\" started", name))
		} else {
			e.Notify.OK(lang.L("Server \"{{.Name}}\" stopped", name))
		}
		e.Reload()
	}()
}

func (c *Card) confirmDelete() {
	e, srv := c.env, c.server

	name := map[string]any{"Name": srv.Name}
	dialogs.Confirm(e.Win, lang.L("Delete server"),
		lang.L("Delete \"{{.Name}}\" and all of its clients?", name),
		func() {
			go func() {
				if err := e.Backend.DeleteServer(srv.ID); err != nil {
					e.Notify.Fail(err)
					return
				}
				e.Notify.OK(lang.L("Server \"{{.Name}}\" deleted", name))
				e.Reload()
			}()
		})
}
