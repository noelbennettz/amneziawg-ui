// Package cliententry is one peer line inside a server card: the avatar and
// presence dot, the name and addresses, the state chip, the live counters and
// the actions behind the overflow menu.
package cliententry

import (
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/browser"
	"amneziawg-web-ui/web-ui/internal/ui/dialogs"
	"amneziawg-web-ui/web-ui/internal/ui/env"
	"amneziawg-web-ui/web-ui/internal/ui/serverlist/serverentry/clientlist/cliententry/clientdialogs"
	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/testhook"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// Row is one peer line. It keeps the labels the traffic feed updates in place,
// so a counter tick never rebuilds the row.
type Row struct {
	env    *env.Env
	server api.Server
	client api.Client

	online    *canvas.Circle
	rx        *canvas.Text
	tx        *canvas.Text
	handshake *canvas.Text
	endpoint  *canvas.Text
	// endpointDot is the separator in front of the endpoint, hidden with it.
	endpointDot *canvas.Text

	body     *widgets.PairBox
	actions  *widgets.SplitRowBox
	counters *widgets.PairBox
	object   fyne.CanvasObject
}

// New renders the row for client, a peer of server.
func New(e *env.Env, server api.Server, client api.Client) *Row {
	r := &Row{
		env:       e,
		server:    server,
		client:    client,
		online:    canvas.NewCircle(style.Muted),
		rx:        widgets.SmallText("—", style.Muted),
		tx:        widgets.SmallText("—", style.Muted),
		handshake: widgets.SmallText(handshakeText("—"), style.Muted),
		endpoint:  widgets.SmallText(endpointText("—"), style.Muted),
	}

	// The avatar is the client's initial on a tonal disc, with the presence
	// light pinned to its corner.
	initial := canvas.NewText(initialOf(client.Name), style.Text)
	initial.TextSize = style.SizeBody

	avatar := container.NewGridWrap(fyne.NewSize(36, 36), container.NewStack(
		roundedRect(style.SurfaceAlt, 18),
		container.NewCenter(initial),
	))

	name := widgets.LabelText(client.Name, style.Text)
	name.TextStyle = fyne.TextStyle{Bold: true}

	meta := widgets.MetaLabel(addresses(client))

	chips := container.NewHBox()
	if client.Status == "suspended" {
		chips.Add(container.NewCenter(widgets.StatusChip(lang.L("suspended"), style.Warning)))
	} else {
		chips.Add(container.NewCenter(widgets.StatusChip(lang.L("active"), style.Success)))
	}
	if client.ApplyISettings {
		chips.Add(container.NewCenter(widgets.Badge("I1-5", style.Accent)))
	}
	if client.SuspendAt != nil {
		when := time.Unix(int64(*client.SuspendAt), 0).Local().Format(clientdialogs.SuspendLayout)
		chips.Add(container.NewCenter(widgets.Badge(lang.L("auto-suspend {{.When}}", map[string]any{"When": when}), style.Error)))
	}

	// The counters are themselves a pair on a phone: the two rates stay on
	// the line they open with, and the handshake and endpoint - which are
	// long, and which nobody reads at a glance - fold under them.
	rates := container.NewHBox(
		container.NewCenter(container.NewGridWrap(fyne.NewSize(9, 9), r.online)),
		container.NewCenter(widgets.SmallIcon(theme.DownloadIcon())), r.rx,
		container.NewCenter(widgets.SmallIcon(theme.UploadIcon())), r.tx,
	)
	// The endpoint is the peer's last observed address: worth having, but it
	// is the one part of the row that nothing else depends on, and on a phone
	// it is the difference between two lines of counters and three. It goes
	// away in the compact layout (see SetNarrow); the config dialog still
	// has it.
	r.endpointDot = widgets.SmallText("·", style.Border)
	when := container.NewHBox(
		widgets.SmallText("·", style.Border),
		r.handshake, r.endpointDot,
		r.endpoint,
	)
	r.counters = widgets.NewPair(style.Space2, 0, rates, when)

	toggleLabel := lang.L("Suspend…")
	if client.Status == "suspended" {
		toggleLabel = lang.L("Activate")
	}

	menu := widgets.MenuButton("client."+client.ID+".menu", []widgets.MenuItem{
		{ID: "qr", Label: lang.L("QR / config"), Do: func() { clientdialogs.ShowConfig(e, server, client) }},
		{ID: "download", Label: lang.L("Download .conf"), Do: func() { browser.OpenURL(e.Backend.ClientConfigURL(server.ID, client.ID)) }},
		{ID: "edit", Label: lang.L("Edit"), Do: func() { clientdialogs.ShowEditor(e, server, &client) }},
		{ID: "toggle", Label: toggleLabel, Do: func() { r.setSuspended(client.Status != "suspended") }},
		{ID: "delete", Label: lang.L("Delete…"), Do: r.confirmDelete},
	})
	testhook.Register("client."+client.ID+".menu", menu)

	// The right-hand side folds on a phone: the state chips and the menu keep
	// the top line, the counters - which say what the peer is doing rather
	// than what it is - take the next one.
	r.actions = widgets.NewSplitRow(style.Space2,
		container.NewCenter(chips),
		container.NewCenter(r.counters),
		container.NewCenter(menu),
	)

	r.body = widgets.NewPair(style.Space2, 44,
		container.NewBorder(nil, nil,
			container.NewHBox(container.NewCenter(avatar), widgets.Gap(style.Space2)),
			nil,
			container.NewVBox(name, meta),
		),
		r.actions,
	)

	r.object = container.NewPadded(container.NewPadded(r.body))
	return r
}

// CanvasObject is the row as the list places it.
func (r *Row) CanvasObject() fyne.CanvasObject { return r.object }

// SetNarrow restacks the row for a compact page and drops the endpoint, which
// is the only part of it that fits nowhere: the row keeps two lines - the
// state and the actions, then the rates and the handshake.
func (r *Row) SetNarrow(narrow bool) {
	r.body.SetNarrow(narrow)
	r.actions.SetNarrow(narrow)
	r.counters.SetNarrow(narrow)

	if narrow {
		r.endpointDot.Hide()
		r.endpoint.Hide()
	} else {
		r.endpointDot.Show()
		r.endpoint.Show()
	}
	r.object.Refresh()
}

// Apply pushes one traffic snapshot into the labels. Must run on the UI
// goroutine.
func (r *Row) Apply(data api.ClientTraffic) bool {
	r.online.FillColor = style.Muted
	if data.Online {
		r.online.FillColor = style.Success
	}
	r.online.Refresh()

	endpoint := data.Endpoint
	if endpoint == "" {
		endpoint = "—"
	}
	handshake := handshakeText(data.LastHandshake)
	changed := r.rx.Text != data.Received || r.tx.Text != data.Sent ||
		r.handshake.Text != handshake || r.endpoint.Text != endpointText(endpoint)

	r.rx.Text = data.Received
	r.rx.Refresh()
	r.tx.Text = data.Sent
	r.tx.Refresh()

	r.handshake.Text = handshake
	r.handshake.Refresh()

	r.endpoint.Text = endpointText(endpoint)
	r.endpoint.Refresh()

	return changed
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func handshakeText(when string) string {
	return lang.L("handshake: {{.When}}", map[string]any{"When": when})
}

func endpointText(endpoint string) string {
	return lang.L("endpoint: {{.Endpoint}}", map[string]any{"Endpoint": endpoint})
}

// addresses is the client's own line: both stacks, the IPv6 half omitted when
// the server predates it.
func addresses(client api.Client) string {
	if client.ClientIPV6 == "" {
		return client.ClientIP
	}
	return client.ClientIP + " · " + client.ClientIPV6
}

// initialOf is the one letter the avatar shows.
func initialOf(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "?"
	}
	return strings.ToUpper(string([]rune(trimmed)[0]))
}

func MonoText(t *canvas.Text) *canvas.Text { return widgets.MonoText(t) }

func roundedRect(fill color.NRGBA, radius float32) fyne.CanvasObject {
	rect := canvas.NewRectangle(fill)
	rect.CornerRadius = radius
	return rect
}

// ── Actions ──────────────────────────────────────────────────────────────────

func (r *Row) setSuspended(suspend bool) {
	e, client := r.env, r.client

	name := map[string]any{"Name": client.Name}
	question := lang.L("Activate \"{{.Name}}\" again?", name)
	if suspend {
		question = lang.L("Suspend \"{{.Name}}\"? The client loses its connection until reactivated.", name)
	}

	dialogs.Confirm(e.Win, lang.L("Change client state"), question, func() {
		go func() {
			var err error
			if suspend {
				err = e.Backend.SuspendClient(r.server.ID, client.ID)
			} else {
				err = e.Backend.ActivateClient(r.server.ID, client.ID)
			}
			if err != nil {
				e.Notify.Fail(err)
				return
			}
			if suspend {
				e.Notify.OK(lang.L("Client \"{{.Name}}\" suspended", name))
			} else {
				e.Notify.OK(lang.L("Client \"{{.Name}}\" activated", name))
			}
			e.Reload()
		}()
	})
}

func (r *Row) confirmDelete() {
	e, client := r.env, r.client

	name := map[string]any{"Name": client.Name}
	dialogs.Confirm(e.Win, lang.L("Delete client"), lang.L("Delete \"{{.Name}}\"?", name), func() {
		go func() {
			if err := e.Backend.DeleteClient(r.server.ID, client.ID); err != nil {
				e.Notify.Fail(err)
				return
			}
			e.Notify.OK(lang.L("Client \"{{.Name}}\" deleted", name))
			e.Reload()
		}()
	})
}
