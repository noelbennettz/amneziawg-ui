// Package ui is the page: a fixed app bar, a scrolling body holding the
// dashboard, the create-server form and the server list, and a snackbar that
// floats over the bottom of the window. It owns the data the body is rendered
// from and the REST polling that keeps it current, and hands everything the
// components need down to them through env.Env.
//
// The tree below this package follows the page top to bottom:
//
//	dashboard                      the accent traffic card and the host gauges
//	newserver                      the collapsible create form
//	serverlist                     the cards
//	  serverentry                  one server
//	    serverdialogs              its overview / raw config dialogs
//	    clientlist                 its peers
//	      cliententry              one peer
//	        clientdialogs          the add/edit form, the config + QR viewer
//
// alongside the leaves every level draws on: style, widgets, dialogs,
// backend, browser and env.
package ui

import (
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"amneziawg-web-ui/web-ui/internal/ui/backend"
	"amneziawg-web-ui/web-ui/internal/ui/dashboard"
	"amneziawg-web-ui/web-ui/internal/ui/env"
	"amneziawg-web-ui/web-ui/internal/ui/newserver"
	"amneziawg-web-ui/web-ui/internal/ui/serverlist"
	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// NewDarkTheme is the page's theme, for main to install before the window
// opens.
func NewDarkTheme() fyne.Theme {
	return style.NewTheme()
}

// UI is the page. It holds the header widgets it updates itself, the two
// body components, and the state behind them.
type UI struct {
	app fyne.App
	win fyne.Window
	env *env.Env

	*state
	feedback *feedback
	snackbar *snackbar

	publicIP   *canvas.Text
	statusSlot *fyne.Container
	summary    *canvas.Text

	// The app bar's pieces, kept so the breakpoint can restack them: the
	// title (and its size), the block holding it with the version under it,
	// and the row of facts at the right.
	title      *canvas.Text
	titleBlock *fyne.Container
	bar        *fyne.Container
	facts      *fyne.Container
	refresh    *widgets.Button

	// narrow is the compact layout the page is in, as last decided from the
	// window width; see reflow.
	narrow bool

	// transport is the status light's label as last shown, so a poll that
	// reports the same state does not lay the bar out again.
	transport string

	charts *dashboard.Panel
	form   *newserver.Form
	list   *serverlist.List
	scroll *container.Scroll
}

// New wires the page up; Build lays it out and Start begins loading.
func New(a fyne.App, w fyne.Window) *UI {
	snack := newSnackbar()
	u := &UI{
		app:      a,
		win:      w,
		state:    newState(),
		feedback: &feedback{win: w, snack: snack},
		snackbar: snack,
	}
	u.env = &env.Env{
		Win:      w,
		Backend:  backend.New(),
		Notify:   u.feedback,
		Reload:   u.reloadServers,
		DefaultI: u.defaultISettings,
	}
	return u
}

// Build assembles the whole page: a fixed app bar, a scrolling body holding
// the dashboard, the create-server form and the server cards, and the snackbar
// layer on top.
func (u *UI) Build() fyne.CanvasObject {
	u.charts = dashboard.New(trafficInterval, u.clampScroll)
	u.form = newserver.New(u.env, u.state, u.clampScroll)
	u.list = serverlist.New(u.env)

	body := container.NewVBox(
		u.charts.CanvasObject(),
		u.form.CanvasObject(),
		u.list.CanvasObject(),
	)

	u.scroll = container.NewVScroll(widgets.Rewrapped(container.NewPadded(body)))

	page := container.NewBorder(u.header(), nil, nil, nil, u.scroll)

	snackLayer := container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(u.snackbar.CanvasObject()),
		container.NewGridWrap(fyne.NewSize(1, style.Space5)),
	)
	return newPage(container.NewStack(page, snackLayer), u.reflow)
}

func (u *UI) header() fyne.CanvasObject {
	u.title = canvas.NewText("AmneziaWG Web UI", style.Text)
	u.title.TextSize = style.SizeDisplay

	version := widgets.SmallText(u.version(), style.Muted)
	version.TextStyle = fyne.TextStyle{Monospace: true}

	u.publicIP = widgets.MonoText(widgets.LabelText(lang.L("unknown"), style.Muted))
	ipChip := widgets.Pill(style.SurfaceAlt, container.NewHBox(
		widgets.LabelText(lang.L("Public IP"), style.Muted),
		container.NewCenter(u.publicIP),
	))

	u.statusSlot = container.NewStack(statusPill(style.Muted, lang.L("connecting…")))

	u.summary = widgets.LabelText("", style.Muted)
	summaryChip := widgets.Pill(style.SurfaceAlt, container.NewCenter(u.summary))

	u.refresh = widgets.NewButton(lang.L("Refresh IP"), theme.ViewRefreshIcon(), u.refreshPublicIP)
	u.refresh.Importance = widget.LowImportance

	// The facts share one line while they fit and fold onto two when they do
	// not: the ip, the status light and the refresh action keep the first
	// line, the summary - the longest and the least urgent - takes the
	// second. See factsLayout.
	u.facts = container.New(&factsLayout{},
		container.NewCenter(ipChip),
		container.NewCenter(u.statusSlot),
		container.NewCenter(summaryChip),
		container.NewCenter(u.refresh),
	)

	// Padding rather than a centre layout: a 28pt canvas.Text needs a few
	// points of slack above it, or the top of the glyphs is cut by the row.
	u.titleBlock = container.NewVBox(
		widgets.Inset(u.title, style.Space1, style.Space1), widgets.Inset(version, style.Space1, 0))

	u.bar = container.New(&barLayout{}, u.titleBlock, u.facts)

	bg := canvas.NewRectangle(style.Surface)
	return container.NewStack(bg, container.NewPadded(u.bar))
}

// barLayout is the app bar: the title block and the facts, side by side with
// the facts at the right edge, or - on a narrow page - the facts under the
// title, which is the compact top app bar of M3.
type barLayout struct {
	narrow bool
}

func (b *barLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}
	titleBlock, facts := objects[0], objects[1]
	titleSize, factsSize := titleBlock.MinSize(), facts.MinSize()

	if b.narrow {
		titleBlock.Move(fyne.NewPos(0, 0))
		titleBlock.Resize(fyne.NewSize(size.Width, titleSize.Height))
		facts.Move(fyne.NewPos(0, titleSize.Height+style.Space2))
		facts.Resize(fyne.NewSize(size.Width, factsSize.Height))
		return
	}

	titleBlock.Move(fyne.NewPos(0, (size.Height-titleSize.Height)/2))
	titleBlock.Resize(fyne.NewSize(size.Width-factsSize.Width-style.Space4, titleSize.Height))
	facts.Move(fyne.NewPos(size.Width-factsSize.Width, (size.Height-factsSize.Height)/2))
	facts.Resize(factsSize)
}

func (b *barLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 2 {
		return fyne.Size{}
	}
	titleSize, factsSize := objects[0].MinSize(), objects[1].MinSize()

	if b.narrow {
		return fyne.NewSize(max(titleSize.Width, factsSize.Width), titleSize.Height+style.Space2+factsSize.Height)
	}
	return fyne.NewSize(titleSize.Width+style.Space4+factsSize.Width, max(titleSize.Height, factsSize.Height))
}

// factsLayout is the row of pills on the right of the app bar: one line when
// there is room, and when there is not the long summary on a line of its own
// under the ip, the status light and the refresh action.
type factsLayout struct {
	narrow bool
}

func (l *factsLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 4 {
		return
	}
	ip, status, summary, refresh := objects[0], objects[1], objects[2], objects[3]
	ipSize, statusSize := ip.MinSize(), status.MinSize()
	refreshSize, summarySize := refresh.MinSize(), summary.MinSize()

	if !l.narrow {
		x := float32(0)
		for _, o := range objects {
			min := o.MinSize()
			o.Move(fyne.NewPos(x, (size.Height-min.Height)/2))
			o.Resize(min)
			x += min.Width + style.Space2
		}
		return
	}

	top := max(ipSize.Height, max(statusSize.Height, refreshSize.Height))
	x := float32(0)
	for _, item := range []struct {
		object fyne.CanvasObject
		size   fyne.Size
	}{{ip, ipSize}, {status, statusSize}, {refresh, refreshSize}} {
		item.object.Move(fyne.NewPos(x, (top-item.size.Height)/2))
		item.object.Resize(item.size)
		x += item.size.Width + style.Space2
	}
	summary.Move(fyne.NewPos(0, top+style.Space2))
	summary.Resize(summarySize)
}

func (l *factsLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 4 {
		return fyne.Size{}
	}
	ipSize, statusSize := objects[0].MinSize(), objects[1].MinSize()
	summarySize, refreshSize := objects[2].MinSize(), objects[3].MinSize()

	if l.narrow {
		top := max(ipSize.Height, max(statusSize.Height, refreshSize.Height))
		return fyne.NewSize(
			max(ipSize.Width+style.Space2+statusSize.Width+style.Space2+refreshSize.Width, summarySize.Width),
			top+style.Space2+summarySize.Height)
	}
	return fyne.NewSize(
		ipSize.Width+statusSize.Width+summarySize.Width+refreshSize.Width+style.Space2*3,
		max(max(ipSize.Height, statusSize.Height), max(summarySize.Height, refreshSize.Height)))
}

// statusPill is the online/offline chip: a dot of the status colour and its
// label, in a tonal pill of that colour.
func statusPill(c color.NRGBA, label string) fyne.CanvasObject {
	dot := canvas.NewCircle(c)
	row := container.NewHBox(
		container.NewCenter(container.NewGridWrap(fyne.NewSize(8, 8), dot)),
		container.NewCenter(widgets.LabelText(label, c)),
	)
	return widgets.Pill(style.WithAlpha(c, 0x22), row)
}

// version is the build stamp "fyne package" bakes into the bundle: the
// release tag it reads from FyneApp.toml - written there from the newest git
// tag by the web-ui make target, and by the image's frontend stage - and the
// build number it bumps on every package run. A bundle built by a plain "go build" carries no metadata at
// all, so it reports itself as a development build rather than claiming a
// version it was never given.
func (u *UI) version() string {
	meta := u.app.Metadata()
	if meta.Version == "" {
		return "dev"
	}

	return fmt.Sprintf("v%s · build %d", meta.Version, meta.Build)
}

// ── Header status ────────────────────────────────────────────────────────────

// setTransport drives the status light: green while the backend answers, red
// once a request has failed and until the next one succeeds.
func (u *UI) setTransport(label string, c color.NRGBA) {
	fyne.Do(func() {
		if u.transport == label {
			return
		}
		u.transport = label
		u.statusSlot.Objects = []fyne.CanvasObject{statusPill(c, label)}
		u.statusSlot.Refresh()
		widgets.Relayout(u.bar)
	})
}

// setSummary shows the server/client counts next to the status light.
func (u *UI) setSummary(text string) {
	fyne.Do(func() {
		if u.summary.Text == text {
			return
		}
		u.summary.Text = text
		u.summary.Refresh()
		widgets.Relayout(u.bar)
	})
}

// unreachable is the shared reaction to a failed REST call.
func (u *UI) unreachable() {
	u.setTransport(lang.L("offline"), style.Error)
	u.setSummary(lang.L("backend unreachable"))
}

func (u *UI) setPublicIP(ip string) {
	if ip == "" {
		ip = lang.L("unknown")
	}
	fyne.Do(func() {
		if u.publicIP.Text == ip {
			return
		}
		u.publicIP.Text = ip
		u.publicIP.Refresh()
		// The bar was laid out with the placeholder's width: the pills after
		// this one would keep their places and be drawn over.
		widgets.Relayout(u.bar)
	})
}

// clampScroll lays the page's content out again and re-checks the scroll
// offset against it. Two things need it: a block that changes height (the
// form collapsing, the list being rebuilt, a row that grew a wrapped line or a
// wider counter), which leaves the viewport parked past the end of the page or
// draws the next card over the taller one - fyne only lays a container out
// when it is resized, and nothing here is resized when its own content grows -
// and the breakpoint flipping, which changes every row's shape without touching
// any size the layout would notice.
func (u *UI) clampScroll() {
	if u.scroll == nil {
		return
	}
	if content := u.scroll.Content; content != nil {
		widgets.Relayout(content)
	}
	u.scroll.Refresh()
}

// ── Compact layout ───────────────────────────────────────────────────────────

// page is the root of the tree: it notices the width the window gives it and
// asks the page to reflow when that width crosses the breakpoint.
//
// A widget, not a layout, because only a canvas object is told its size after
// the window has resized. A layout's minimum size is what decided that size in
// the first place, so a layout that reported the narrow minimum would also be
// given the narrow height on a desktop window - and one that reported the wide
// minimum would be laid out wider than a phone and clipped by it. The width the
// window hands over is the one fact that is true either way.
type page struct {
	widget.BaseWidget

	content fyne.CanvasObject
	reflow  func(width float32)

	narrow bool
	known  bool
}

// The smallest page the root admits to: below this the window itself is the
// problem, and the layout below this one still lays out at whatever it is given.
const (
	minPageWidth  float32 = 320
	minPageHeight float32 = 320
)

func newPage(content fyne.CanvasObject, reflow func(width float32)) *page {
	p := &page{content: content, reflow: reflow}
	p.ExtendBaseWidget(p)
	return p
}

func (p *page) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.content)
}

// Resize is where the breakpoint is read. The reflow itself is put off by one
// tick: it replaces the layouts of containers below this one, and those are
// being laid out right now.
func (p *page) Resize(size fyne.Size) {
	p.BaseWidget.Resize(size)

	narrow := size.Width < style.Breakpoint
	if p.known && narrow == p.narrow {
		return
	}
	p.narrow, p.known = narrow, true

	width := size.Width
	time.AfterFunc(0, func() { fyne.Do(func() { p.reflow(width) }) })
}

// MinSize is what the page claims as its own: almost nothing, on purpose.
//
// The page is the window's content. It is exactly as big as the window and its
// children fold to fit it (see style.Breakpoint), so the content's own minimum
// is none of the window's business - and reporting it is what broke a browser
// window narrower than the two-column layout: fyne grows its canvas to at least
// the content's minimum (glCanvas.SetContent), so a page carrying the 700
// points of its wide layout made the canvas 700 points wide inside a 588-point
// window. From then on the page was laid out at 700 and could never see that
// the window was narrow, which is the one input the breakpoint is made of; the
// cards to the right of the window were simply clipped away.
func (p *page) MinSize() fyne.Size {
	return fyne.NewSize(minPageWidth, minPageHeight)
}

// reflow switches the page between the two-column layout and the compact one.
// Every part that owns a row with something on each side has to be told; the
// scroll is refreshed last, so the taller stacked content is measured before
// the viewport is re-clamped against it.
func (u *UI) reflow(width float32) {
	u.narrow = width < style.Breakpoint

	bar := u.bar.Layout.(*barLayout)
	bar.narrow = u.narrow
	u.facts.Layout.(*factsLayout).narrow = u.narrow

	// M3's compact top app bar sets its title at 22, not at the 28 of a
	// desktop window, and drops the labels of its actions down to icons: on
	// a phone the three facts of the bar have to share 390 points between
	// them.
	u.title.TextSize = style.SizeDisplay
	u.refresh.SetText(lang.L("Refresh IP"))
	if u.narrow {
		u.title.TextSize = style.SizeTitle
		u.refresh.SetText("")
	}
	u.title.Refresh()

	u.bar.Refresh()
	u.charts.SetNarrow(u.narrow)
	u.form.SetNarrow(u.narrow)
	u.list.SetNarrow(u.narrow)
	u.clampScroll()
}
