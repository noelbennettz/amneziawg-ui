// Package newserver is the collapsible "Create New VPN Server" panel at the
// top of the page.
package newserver

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/env"
	"amneziawg-web-ui/web-ui/internal/ui/testhook"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// What the engine accepts, and the checks against it, live in the shared api
// package next to the types they describe - the form and the backend call the
// same copy. What stays here is presentation: the recommended values the
// captions carry, as the amneziawg-linux-kernel-module README gives them.

// The values a new server starts from when the backend reports none of its
// own, which is the case only until /api/system/status has answered. The
// operator overrides them with DEFAULT_PORT, DEFAULT_SUBNET, DEFAULT_DNS and
// AUTO_START_SERVERS, the same way DEFAULT_MTU overrides the MTU; see State.
const (
	DefaultPort   = 54844
	DefaultSubnet = "10.0.0.0/24"
	DefaultDNS    = "8.8.8.8,1.1.1.1"
	// DefaultAutoStart is what a new server does about starting: the panel
	// has always brought one up as it is created, and AUTO_START_SERVERS=false
	// is what turns that off.
	DefaultAutoStart = true
)

// DefaultMTU is the MTU used until /api/system/status reports what the
// backend was configured with; the range it has to be in lives in api. The
// page's State falls back to it, which is why it is exported.
const DefaultMTU = 1420

// State is what the form reads from the page: what a new server must not
// collide with, and what it starts from. All three are safe from any
// goroutine.
type State interface {
	// ServerMTU is the MTU a new server starts from: what the backend was
	// configured with, or a built-in default while that is unknown.
	ServerMTU() int
	// TakenPorts maps every port already spoken for to what holds it: an
	// existing server, or the panel's own listener.
	TakenPorts() map[int]string
	// TakenSubnets maps every subnet an existing server occupies to that
	// server.
	TakenSubnets() map[string]string
	// TakenSubnetsV6 does the same for the IPv6 half: every ULA /64 an
	// existing server owns.
	TakenSubnetsV6() map[string]string

	// DefaultPort, DefaultSubnet and DefaultDNS are what the backend was
	// started with: the port, subnet and DNS servers a new server starts
	// from. Each falls back to the built-in default while the status call is
	// still in flight.
	DefaultPort() int
	DefaultSubnet() string
	DefaultDNS() string
	// DefaultAutoStart is AUTO_START_SERVERS: whether a server created here
	// is brought up as soon as it exists. It falls back the same way.
	DefaultAutoStart() bool
}

// Form is the panel. It has two modes: the default one asks for a name and a
// port and generates everything else, the advanced one exposes every knob
// (see setAdvanced).
type Form struct {
	env   *env.Env
	state State

	// reflow tells the page its content height changed, so it can re-clamp
	// the scroll offset; see toggleOpen.
	reflow func()

	name     *widget.Entry
	port     *widget.Entry
	subnet   *widget.Entry
	subnetV6 *widget.Entry
	mtu      *widget.Entry
	dns      *widget.Entry
	endpoint *widget.Entry

	// mtuSeed is the last MTU the form itself wrote into the field, so it can
	// tell its own value from one the user typed. The three others do the same
	// for the fields the backend has a default for.
	mtuSeed       string
	portSeed      string
	subnetSeed    string
	dnsSeed       string
	autoStartSeed bool

	advancedMode *widgets.Switch
	extras       *fyne.Container
	simpleNote   *widget.Label

	autoStart   *widgets.Switch
	obfuscation *widgets.Switch

	jc, jmin, jmax *widget.Entry
	s1, s2, s3, s4 *widget.Entry
	h1, h2, h3, h4 *widget.Entry

	distinctPaddings *widgets.Switch
	randomTrailers   *widgets.Switch
	disableCookies   *widgets.Switch
	headerKey        *widget.Entry

	advanced []*advancedField

	obfCard fyne.CanvasObject
	errors  *widget.Label
	create  *widgets.Button

	toggle *widgets.Button
	body   *fyne.Container
	panel  *fyne.Container

	// grids are the field grids whose column count depends on the window
	// width: two fields side by side on a desktop, one on a phone.
	grids  []formGrid
	narrow bool
}

// formGrid is one grid of fields and the column counts it uses in either
// layout.
type formGrid struct {
	box          *fyne.Container
	wide, narrow int
}

// SetNarrow narrows every field grid to the column count the compact page has
// room for.
func (f *Form) SetNarrow(narrow bool) {
	if f.narrow == narrow {
		return
	}
	f.narrow = narrow

	for _, g := range f.grids {
		columns := g.wide
		if narrow {
			columns = g.narrow
		}
		g.box.Layout = layout.NewGridLayout(columns)
		g.box.Refresh()
	}
	f.panel.Refresh()
}

// advancedField is one of the optional AWG 3.x per-side timing knobs.
type advancedField struct {
	key string
	// limits is the accepted range, shown in the caption; hint is the engine
	// default plus an example, shown as the placeholder once the field is
	// emptied.
	limits string
	hint   string
	entry  *widget.Entry
}

// New builds the panel. reflow is called whenever it changes height.
func New(e *env.Env, state State, reflow func()) *Form {
	f := &Form{env: e, state: state, reflow: reflow}

	f.name = widgets.EntryWithPlaceholder(lang.L("My VPN Server"))
	f.portSeed = strconv.Itoa(state.DefaultPort())
	f.port = widgets.NumberEntry(f.portSeed, "1-65535")
	f.subnetSeed = state.DefaultSubnet()
	f.subnet = widgets.EntryWithText(f.subnetSeed)
	f.subnetV6 = widgets.EntryWithPlaceholder(lang.L("automatic: a free ULA /64, e.g. fd42:ab12::/64"))
	f.mtuSeed = strconv.Itoa(state.ServerMTU())
	f.mtu = widgets.NumberEntry(f.mtuSeed, fmt.Sprintf("%d-%d", api.MinMTU, api.MaxMTU))

	f.dnsSeed = state.DefaultDNS()
	f.dns = widgets.EntryWithText(f.dnsSeed)
	f.endpoint = widgets.EntryWithPlaceholder(lang.L("leave empty to use the auto-detected public IP"))

	f.advancedMode = widgets.NewSwitch(false, f.setAdvanced)
	f.autoStartSeed = state.DefaultAutoStart()
	f.autoStart = widgets.NewSwitch(f.autoStartSeed, nil)
	f.obfuscation = widgets.NewSwitch(true, func(bool) { f.showObfuscation() })

	// The two fields the simple mode asks for sit side by side; the rest of
	// them only exist in the advanced mode and keep the usual form rows.
	basics := container.NewGridWithColumns(2,
		widgets.Labeled(lang.L("Server name"), f.name),
		widgets.Labeled(lang.L("Port (1-65535)"), f.port),
	)
	f.grids = append(f.grids, formGrid{box: basics, wide: 2, narrow: 1})

	extrasGrid := container.NewGridWithColumns(2,
		widgets.Field(lang.L("IPv4 subnet"), f.subnet, lang.L("e.g. 10.0.0.0/24")),
		widgets.Field(lang.L("IPv6 subnet"), f.subnetV6,
			lang.L("ULA /64, e.g. fd42:ab12::/64; empty picks a free one. Clients get an address here and reach IPv6 through NAT6.")),
		widgets.Field(lang.L("MTU (recommended 1280)"), f.mtu,
			lang.L("1280-1440. AmneziaWG 3.x pads every packet, so above about 1425 a full-size one no longer fits a 1500-byte path")),
		widgets.Field(lang.L("DNS servers"), f.dns, lang.L("comma-separated IPs, e.g. 8.8.8.8,1.1.1.1")),
		widgets.Field(lang.L("Endpoint"), f.endpoint, lang.L("custom IP or hostname clients connect to")),
		container.NewVBox(
			widgets.ToggleRow(lang.L("Auto-start server on creation"), f.autoStart),
			widgets.ToggleRow(lang.L("Enable traffic obfuscation (AmneziaWG 3.1)"), f.obfuscation),
		),
	)
	f.grids = append(f.grids, formGrid{box: extrasGrid, wide: 2, narrow: 1})
	f.extras = container.NewVBox(extrasGrid)

	f.simpleNote = widget.NewLabel(lang.L("Subnet, MTU, DNS, endpoint and the AmneziaWG 3.1 obfuscation parameters are " +
		"generated automatically. Tick \"Advanced settings\" to fill them in yourself."))
	f.simpleNote.Wrapping = fyne.TextWrapWord
	f.simpleNote.TextStyle = fyne.TextStyle{Italic: true}

	f.errors = widget.NewLabel("")
	f.errors.Wrapping = fyne.TextWrapWord
	f.errors.Importance = widget.DangerImportance
	f.errors.Hide()

	f.create = widgets.NewButton(lang.L("Create server"), theme.ConfirmIcon(), f.submit)
	f.create.Importance = widget.HighImportance

	f.obfCard = f.buildObfuscation()

	content := container.NewVBox(
		basics,
		widgets.ToggleRow(lang.L("Advanced settings"), f.advancedMode),
		f.simpleNote,
		f.extras,
		f.obfCard,
		f.errors,
		container.NewHBox(layout.NewSpacer(), f.create),
	)

	// Start collapsed down to name and port; setAdvanced owns every piece of
	// the layout the two modes disagree about.
	f.setAdvanced(false)

	// A hand-rolled disclosure rather than widget.Accordion: collapsing has
	// to tell the page to re-clamp its scroll offset, otherwise the viewport
	// can stay parked below the end of the shortened content.
	f.body = container.NewPadded(content)
	f.body.Hide()

	f.toggle = widgets.NewButton(lang.L("Create New VPN Server"), theme.MenuDropDownIcon(), f.toggleOpen)
	f.toggle.Alignment = widget.ButtonAlignLeading
	f.toggle.Importance = widget.LowImportance

	f.panel = container.NewVBox(f.toggle, f.body)

	// Names the browser suite uses to reach the form: a coordinate would move
	// the moment a field is added or a hint grows a line.
	for id, obj := range map[string]fyne.CanvasObject{
		"form.toggle":   f.toggle,
		"form.advanced": f.advancedMode,
		"form.name":     f.name,
		"form.port":     f.port,
		"form.subnet":   f.subnet,
		"form.subnetV6": f.subnetV6,
		"form.mtu":      f.mtu,
		"form.dns":      f.dns,
		"form.endpoint": f.endpoint,
		"form.create":   f.create,
	} {
		testhook.Register(id, obj)
	}

	return f
}

// CanvasObject is the panel as the page places it.
func (f *Form) CanvasObject() fyne.CanvasObject {
	return container.NewPadded(widgets.Card(f.panel))
}

func (f *Form) toggleOpen() {
	if f.body.Visible() {
		f.collapse()
		return
	}

	// Take the backend's defaults first, then offer a port and a subnet nothing
	// is using yet, so creating a second server is still a matter of typing a
	// name and pressing the button.
	f.SeedDefaults()
	f.seedPort()
	f.seedSubnet()
	f.seedMTU()

	f.body.Show()
	f.toggle.SetIcon(theme.MenuDropUpIcon())
	f.reflow()
}

// SeedDefaults fills the fields the backend has a default for - a new server's
// port, subnet and DNS - with what /api/system/status reported, which is what
// DEFAULT_PORT, DEFAULT_SUBNET and DEFAULT_DNS were read into. The widgets are
// built before that call lands, so without this the panel would keep its own
// numbers and an operator's settings would never reach a server created here.
//
// Like seedMTU it only overwrites a value the form itself wrote: anything typed
// over stays.
func (f *Form) SeedDefaults() {
	f.seed(f.port, &f.portSeed, strconv.Itoa(f.state.DefaultPort()))
	f.seed(f.subnet, &f.subnetSeed, f.state.DefaultSubnet())
	f.seed(f.dns, &f.dnsSeed, f.state.DefaultDNS())
	f.seedSwitch(f.autoStart, &f.autoStartSeed, f.state.DefaultAutoStart())
}

func (f *Form) seed(entry *widget.Entry, written *string, provided string) {
	if provided == "" || entry.Text != *written {
		return
	}
	*written = provided
	entry.SetText(provided)
}

// seedSwitch is seed for a control that is on or off rather than typed into.
func (f *Form) seedSwitch(sw *widgets.Switch, written *bool, provided bool) {
	if sw.Checked() != *written {
		return
	}
	*written = provided
	sw.SetChecked(provided)
}

// seedPort puts the first free port at or above the backend's default into the
// field, and records it as the form's own.
//
// Recording it is the whole point: a value the form picked but did not claim
// looks like something the operator typed, and SeedDefaults then leaves it
// alone for the rest of the page's life - which is how a changed DEFAULT_PORT
// stopped reaching the form as soon as one server existed.
func (f *Form) seedPort() {
	f.portSeed = f.nextFreePort()
	f.port.SetText(f.portSeed)
}

// seedSubnet does the same for a subnet an existing server already holds.
func (f *Form) seedSubnet() {
	if _, taken := f.subnetHolder(f.subnet.Text); !taken {
		return
	}
	f.subnetSeed = f.nextFreeSubnet()
	f.subnet.SetText(f.subnetSeed)
}

// seedMTU puts the MTU the backend was configured with into the form, along
// with paddings that fit it.
//
// The widgets are built before /api/system/status has answered, so the first
// open is where the operator's DEFAULT_MTU can first be honoured. It only ever
// overwrites its own last value: once the MTU has been typed over, the form is
// the user's and reopening it must not undo that.
func (f *Form) seedMTU() {
	if f.mtu.Text != f.mtuSeed {
		return
	}

	mtu := f.state.ServerMTU()
	f.mtuSeed = strconv.Itoa(mtu)
	f.mtu.SetText(f.mtuSeed)

	padding, _, _, _ := api.RecommendedPaddings(mtu, false)
	for _, entry := range []*widget.Entry{f.s1, f.s2, f.s3, f.s4} {
		entry.SetText(strconv.Itoa(padding))
	}
}

func (f *Form) collapse() {
	f.body.Hide()
	f.toggle.SetIcon(theme.MenuDropDownIcon())
	f.reflow()
}

// setAdvanced switches between the two modes: the simple one shows a name and
// a port and lets build() generate the rest, the advanced one adds the
// remaining fields, the toggles and the obfuscation block.
func (f *Form) setAdvanced(on bool) {
	if on {
		f.simpleNote.Hide()
		f.extras.Show()
	} else {
		f.simpleNote.Show()
		f.extras.Hide()
	}

	f.showObfuscation()
	f.reflow()
}

// showObfuscation keeps the parameter block visible only where it can be
// edited: in advanced mode, with obfuscation turned on. The nil check covers
// the SetChecked calls newServerForm makes before the block itself is built.
func (f *Form) showObfuscation() {
	if f.obfCard == nil {
		return
	}
	if f.advancedMode.Checked() && f.obfuscation.Checked() {
		f.obfCard.Show()
	} else {
		f.obfCard.Hide()
	}
}

// buildObfuscation lays out the AmneziaWG parameter block: the packet-shaping
// numbers first, then the 3.1 switches, the header protection key and the
// optional per-side timing knobs.
func (f *Form) buildObfuscation() fyne.CanvasObject {
	// The paddings start on a value the generator would pick for the MTU in
	// the field above rather than on a fixed number, so opening the advanced
	// mode and pressing Create straight away cannot hand out a padding too
	// big for that MTU.
	seed, _, _, _ := api.RecommendedPaddings(f.state.ServerMTU(), false)
	padding := strconv.Itoa(seed)

	f.jc = widgets.NumberEntry("8", "1-65535")
	f.jmin = widgets.NumberEntry("8", "1-65535")
	f.jmax = widgets.NumberEntry("80", "1-65535")
	f.s1 = widgets.NumberEntry(padding, "12-65535")
	f.s2 = widgets.NumberEntry(padding, "12-65535")
	f.s3 = widgets.NumberEntry(padding, "12-65535")
	f.s4 = widgets.NumberEntry(padding, "12-65535")
	f.h1 = widgets.NumberEntry("1", "1-4294967295")
	f.h2 = widgets.NumberEntry("2", "1-4294967295")
	f.h3 = widgets.NumberEntry("3", "1-4294967295")
	f.h4 = widgets.NumberEntry("4", "1-4294967295")

	// Captions carry the recommended value, placeholders the accepted range:
	// the range only matters once you are deliberately leaving the
	// recommendation, and the field shows it as soon as it is cleared.
	junk := container.NewGridWithColumns(3,
		widgets.Labeled(lang.L("Jc (recommended 4-12)"), f.jc),
		widgets.Labeled(lang.L("Jmin (recommended 8)"), f.jmin),
		widgets.Labeled(lang.L("Jmax (recommended 80)"), f.jmax),
	)
	f.grids = append(f.grids, formGrid{box: junk, wide: 3, narrow: 1})
	sizes := container.NewGridWithColumns(4,
		widgets.Labeled(lang.L("S1 (recommended 15-150)"), f.s1),
		widgets.Labeled(lang.L("S2 (same as S1)"), f.s2),
		widgets.Labeled(lang.L("S3 (same as S1)"), f.s3),
		widgets.Labeled(lang.L("S4 (same as S1)"), f.s4),
	)
	f.grids = append(f.grids, formGrid{box: sizes, wide: 4, narrow: 1})
	headers := container.NewGridWithColumns(4,
		widgets.Labeled(lang.L("H1 (recommended 1)"), f.h1),
		widgets.Labeled(lang.L("H2 (recommended 2)"), f.h2),
		widgets.Labeled(lang.L("H3 (recommended 3)"), f.h3),
		widgets.Labeled(lang.L("H4 (recommended 4)"), f.h4),
	)
	f.grids = append(f.grids, formGrid{box: headers, wide: 4, narrow: 1})
	f.distinctPaddings = widgets.NewSwitch(false, nil)

	sizesNote := widgets.MutedNote(lang.L("At least 12 for each padding is the one hard rule here - header protection is always " +
		"on in this app and takes its 12-byte nonce from the start of that padding. H1-H4 at 1/2/3/4 follows from the " +
		"same thing: header protection encrypts the message type itself, so custom headers change nothing an observer " +
		"can see (without header protection the advice is the opposite - four distinct values in 5-2147483647).\n\n" +
		"One value for all four paddings is what the docs recommend while RandomTrailers is on, because it is what keeps " +
		"the receiver from reading one message type as another. It does leave the gaps between the types at WireGuard's " +
		"own 148/92/64/32, which an observer can recover from the smallest packet of each type; rolling them separately " +
		"hides that, and gives up the receiver's margin in exchange."))

	limitsNote := widgets.MutedNote(lang.L("The placeholders are what the engine accepts, not what is wise. On top of them S1 must " +
		"fit MTU-148, S2 must fit MTU-92, S1+56 must not equal S2, and H1-H4 must all differ."))

	random := widgets.NewButton(lang.L("Generate random parameters"), theme.ViewRefreshIcon(), f.randomise)

	f.randomTrailers = widgets.NewSwitch(true, nil)
	f.disableCookies = widgets.NewSwitch(true, nil)

	togglesNote := widgets.MutedNote(lang.L("AmneziaWG 3.1 packet shaping. Unlike the timing knobs below, both switches are written " +
		"identically into the server config and every client config - the two sides must agree, so turn them off if any of " +
		"your clients is older than AmneziaWG 3.1 (AmneziaVPN < 5.0.1.5)."))

	f.headerKey = widgets.EntryWithPlaceholder(lang.L("auto-generated if left empty"))
	keyNote := widgets.MutedNote(lang.L("AmneziaWG 3.x requires S1-S4 >= 12 and this key to match byte-for-byte between the server " +
		"and every client config."))

	// The units in the limits and the wording of the hints are what changes
	// between languages; the keys are the config file's own and stay.
	f.advanced = []*advancedField{
		{key: "ContentPaddingAddition", limits: "0-65535", hint: lang.L("off by default, e.g. 0-64")},
		{key: "RekeyAfterTime", limits: lang.L("0-65535 s"), hint: lang.L("default 120, e.g. 100-140")},
		{key: "RekeyTimeout", limits: lang.L("0-65535 s"), hint: lang.L("default 5, e.g. 4-7")},
		{key: "RejectAfterTime", limits: lang.L("0-65535 s"), hint: lang.L("default 180, e.g. 160-200")},
		{key: "KeepaliveTimeout", limits: lang.L("0-65535 s"), hint: lang.L("default 10, e.g. 8-12")},
		{key: "MaxHandshakeAttempts", limits: "0-65535", hint: lang.L("default 18, e.g. 14-20")},
		{key: "PersistentKeepalive", limits: lang.L("0-65535 s"), hint: lang.L("default 25, e.g. 22-30")},
	}
	advancedGrid := container.NewGridWithColumns(2)
	for _, field := range f.advanced {
		field.entry = widgets.EntryWithPlaceholder(field.hint)
		advancedGrid.Add(widgets.Labeled(fmt.Sprintf("%s (%s)", field.key, field.limits), field.entry))
	}
	f.grids = append(f.grids, formGrid{box: advancedGrid, wide: 2, narrow: 1})

	advancedNote := widgets.MutedNote(lang.L("Optional AWG 3.x timing knobs (an integer or an \"a-b\" range). They are applied " +
		"per side and do not need to match between server and client; leave empty to use the engine defaults."))

	box := container.NewVBox(
		widgets.SectionTitle(lang.L("Obfuscation parameters")),
		junk, sizes, widgets.ToggleRow(lang.L("Roll S1-S4 separately instead of one value for all four"), f.distinctPaddings), headers, sizesNote, limitsNote,
		container.NewHBox(random),
		widgets.Separator(),
		togglesNote,
		widgets.ToggleRow(lang.L("RandomTrailers - append a random number of bytes to every packet"), f.randomTrailers),
		widgets.ToggleRow(lang.L("DisableCookies - never answer with cookie replies, skip under-load MAC2 checks"), f.disableCookies),
		widgets.Separator(),
		widgets.Labeled(lang.L("HeaderProtectionKey (base64)"), f.headerKey), keyNote,
		widgets.Separator(),
		advancedNote, advancedGrid,
	)

	return container.NewPadded(widgets.Card(box))
}

// randomise rolls the packet-shaping numbers, leaving the fields the user is
// more likely to have deliberately set (Jmin/Jmax, the switches, the timing
// knobs) alone.
func (f *Form) randomise() {
	p := api.GenerateObfuscation(f.mtuOrDefault(), f.distinctPaddings.Checked())

	f.jc.SetText(strconv.Itoa(p.Jc))
	f.s1.SetText(strconv.Itoa(p.S1))
	f.s2.SetText(strconv.Itoa(p.S2))
	f.s3.SetText(strconv.Itoa(p.S3))
	f.s4.SetText(strconv.Itoa(p.S4))
	f.h1.SetText(strconv.Itoa(p.H1))
	f.h2.SetText(strconv.Itoa(p.H2))
	f.h3.SetText(strconv.Itoa(p.H3))
	f.h4.SetText(strconv.Itoa(p.H4))
}

// mtuOrDefault reads the MTU field for the callers that only need a plausible
// number (parameter generation); build() does the reporting parse.
func (f *Form) mtuOrDefault() int {
	mtu, err := strconv.Atoi(strings.TrimSpace(f.mtu.Text))
	if err != nil || mtu < api.MinMTU || mtu > api.MaxMTU {
		return f.state.ServerMTU()
	}
	return mtu
}

// nextFreePort walks up from whatever the field holds until it finds a port no
// server has taken. An unparsable value is left alone - build() reports it.
func (f *Form) nextFreePort() string {
	port, err := strconv.Atoi(strings.TrimSpace(f.port.Text))
	if err != nil || port < api.MinPort || port > api.MaxPort {
		return f.port.Text
	}

	taken := f.state.TakenPorts()
	for ; port <= api.MaxPort; port++ {
		if _, used := taken[port]; !used {
			return strconv.Itoa(port)
		}
	}
	return f.port.Text
}

// nextFreeSubnet picks a /24 that overlaps no existing server's subnet, so
// servers created in the simple mode never collide with each other.
//
// The operator's DEFAULT_SUBNET is the first answer while it is free, and its
// neighbours - the other 255 /24s of the same first two octets - come next.
// With 10.9.0.0/24 configured, a second server has to land on 10.9.1.0/24:
// walking the 10.x.x.0 block from 10.0.0.0 instead put every server after the
// first on the built-in default, which reads as "the setting did nothing".
func (f *Form) nextFreeSubnet() string {
	preferred := f.state.DefaultSubnet()
	if preferred != "" {
		if _, taken := f.subnetHolder(preferred); !taken {
			return preferred
		}
		if neighbour, ok := f.nextFreeNeighbour(preferred); ok {
			return neighbour
		}
	}
	// The 10.x block the simple mode started on, for a DEFAULT_SUBNET that is
	// not a /24 or whose neighbours are all taken: a free /24 is better than
	// a collision, and the backend accepts any IPv4 subnet.
	for i := range 256 {
		candidate := fmt.Sprintf("10.%d.0.0/24", i)
		if _, taken := f.subnetHolder(candidate); !taken {
			return candidate
		}
	}
	return preferred
}

// nextFreeNeighbour walks the /24s that share the operator's first two octets,
// starting at the one they configured and wrapping once round the /16, and
// returns the first one no server holds.
func (f *Form) nextFreeNeighbour(preferred string) (string, bool) {
	network, err := api.ParseSubnet(strings.TrimSpace(preferred))
	if err != nil {
		return "", false
	}
	// Only a /24 has siblings a new server can move to without cutting the
	// operator's block in half; anything wider falls through to the 10.x
	// search.
	if ones, _ := network.Mask.Size(); ones != 24 {
		return "", false
	}

	block := network.IP.To4()
	for offset := range 256 {
		third := (int(block[2]) + offset) % 256
		candidate := fmt.Sprintf("%d.%d.%d.0/24", block[0], block[1], third)
		if _, taken := f.subnetHolder(candidate); !taken {
			return candidate, true
		}
	}
	return "", false
}

// subnetHolder names the existing server whose subnet shares addresses with
// this one, the way the backend's check will.
func (f *Form) subnetHolder(subnet string) (string, bool) {
	for taken, holder := range f.state.TakenSubnets() {
		if api.SubnetsOverlap(strings.TrimSpace(subnet), taken) {
			return holder, true
		}
	}
	return "", false
}

// subnetV6Holder is subnetHolder for the IPv6 half, against the ULA /64s the
// other servers own.
func (f *Form) subnetV6Holder(subnetV6 string) (string, bool) {
	for taken, holder := range f.state.TakenSubnetsV6() {
		if api.SubnetsOverlap(strings.TrimSpace(subnetV6), taken) {
			return holder, true
		}
	}
	return "", false
}

func (f *Form) showErrors(messages []string) {
	if len(messages) == 0 {
		f.errors.Hide()
		return
	}
	f.errors.SetText(strings.Join(messages, "\n"))
	f.errors.Show()
}

// build validates the fields the current mode exposes and returns the payload
// for POST /api/servers.
func (f *Form) build() (api.CreateServerRequest, []string) {
	var problems []string

	name := strings.TrimSpace(f.name.Text)
	if name == "" {
		problems = append(problems, lang.L("Server name is required"))
	}

	port, err := strconv.Atoi(strings.TrimSpace(f.port.Text))
	switch taken, used := f.state.TakenPorts()[port]; {
	case err != nil, port < api.MinPort, port > api.MaxPort:
		problems = append(problems, lang.L("Port must be between 1 and 65535"))
	case used:
		problems = append(problems, lang.L("Port {{.Port}} is already used by {{.Holder}}", map[string]any{"Port": port, "Holder": taken}))
	}

	if !f.advancedMode.Checked() {
		return f.generated(name, port), problems
	}

	autoStart := f.autoStart.Checked()
	obfuscation := f.obfuscation.Checked()
	req := api.CreateServerRequest{
		Name:        name,
		Port:        port,
		Subnet:      strings.TrimSpace(f.subnet.Text),
		SubnetV6:    strings.TrimSpace(f.subnetV6.Text),
		Endpoint:    strings.TrimSpace(f.endpoint.Text),
		DNS:         strings.TrimSpace(f.dns.Text),
		AutoStart:   &autoStart,
		Obfuscation: &obfuscation,
	}

	if _, err := api.ParseSubnet(req.Subnet); err != nil {
		problems = append(problems, lang.L("Valid subnet is required (e.g. 10.0.0.0/24)"))
	} else if holder, taken := f.subnetHolder(req.Subnet); taken {
		problems = append(problems, lang.L("Subnet {{.Subnet}} overlaps {{.Holder}}", map[string]any{"Subnet": req.Subnet, "Holder": holder}))
	}

	// Empty means "let the backend pick a free ULA /64", which is what the
	// simple mode and most operators want; a value is checked here exactly as
	// the backend will check it.
	if req.SubnetV6 != "" {
		if _, err := api.ParseSubnetV6(req.SubnetV6); err != nil {
			problems = append(problems, lang.L("Valid IPv6 subnet is required (e.g. fd42:ab12::/64), or leave it empty"))
		} else if holder, taken := f.subnetV6Holder(req.SubnetV6); taken {
			problems = append(problems, lang.L("IPv6 subnet {{.Subnet}} overlaps {{.Holder}}", map[string]any{"Subnet": req.SubnetV6, "Holder": holder}))
		}
	}

	mtu, err := strconv.Atoi(strings.TrimSpace(f.mtu.Text))
	if err != nil || mtu < api.MinMTU || mtu > api.MaxMTU {
		problems = append(problems, lang.L("MTU must be between {{.Min}} and {{.Max}}", map[string]any{"Min": api.MinMTU, "Max": api.MaxMTU}))
	}
	req.MTU = mtu

	servers := splitList(strings.TrimSpace(f.dns.Text))
	if len(servers) == 0 {
		problems = append(problems, lang.L("At least one DNS server is required"))
	}
	for _, dns := range servers {
		// Either stack: a resolver the client cannot reach is no resolver.
		if !api.IsIPAddress(dns) {
			problems = append(problems, lang.L("Invalid DNS server IP: {{.IP}}", map[string]any{"IP": dns}))
			break
		}
	}

	// Empty means "use the address the backend detected", which is what the
	// placeholder says; a value has to be a host on its own, exactly as the
	// backend will read it - the port is the server's and is appended there.
	if req.Endpoint != "" {
		if _, err := api.ParseEndpoint(req.Endpoint); err != nil {
			problems = append(problems, lang.L("Invalid endpoint: {{.Endpoint}}", map[string]any{"Endpoint": req.Endpoint}))
		}
	}

	if obfuscation {
		params, issues := f.obfuscationParams(mtu)
		problems = append(problems, issues...)
		req.ObfuscationParams = params
	}

	return req, problems
}

// generated is the simple mode's payload: the name and port the user gave,
// everything else derived. The advanced entries are deliberately not read -
// switching back to the simple mode means "forget what I typed there", not
// "keep it but hide it".
func (f *Form) generated(name string, port int) api.CreateServerRequest {
	autoStart, obfuscation := f.state.DefaultAutoStart(), true
	mtu := f.state.ServerMTU()
	return api.CreateServerRequest{
		Name:      name,
		Port:      port,
		Subnet:    f.nextFreeSubnet(),
		MTU:       mtu,
		DNS:       f.state.DefaultDNS(),
		Endpoint:  "", // the backend fills in the detected public IP
		AutoStart: &autoStart,
		// The simple mode has no switch to offer, so it takes the shape the
		// AmneziaWG docs recommend: one padding for all four message types.
		Obfuscation:       &obfuscation,
		ObfuscationParams: api.GenerateObfuscation(mtu, false),
	}
}

func (f *Form) obfuscationParams(mtu int) (*api.ObfuscationParams, []string) {
	var problems []string

	number := func(entry *widget.Entry, label string) int {
		value, err := strconv.Atoi(strings.TrimSpace(entry.Text))
		if err != nil {
			problems = append(problems, lang.L("{{.Field}} must be a number", map[string]any{"Field": label}))
			return 0
		}
		return value
	}

	params := &api.ObfuscationParams{
		Jc:                  number(f.jc, "Jc"),
		Jmin:                number(f.jmin, "Jmin"),
		Jmax:                number(f.jmax, "Jmax"),
		S1:                  number(f.s1, "S1"),
		S2:                  number(f.s2, "S2"),
		S3:                  number(f.s3, "S3"),
		S4:                  number(f.s4, "S4"),
		H1:                  number(f.h1, "H1"),
		H2:                  number(f.h2, "H2"),
		H3:                  number(f.h3, "H3"),
		H4:                  number(f.h4, "H4"),
		RandomTrailers:      f.randomTrailers.Checked(),
		DisableCookies:      f.disableCookies.Checked(),
		HeaderProtectionKey: strings.TrimSpace(f.headerKey.Text),
	}

	for _, field := range f.advanced {
		value := strings.TrimSpace(field.entry.Text)
		if value == "" {
			continue
		}
		switch field.key {
		case "ContentPaddingAddition":
			params.ContentPaddingAddition = value
		case "RekeyAfterTime":
			params.RekeyAfterTime = value
		case "RekeyTimeout":
			params.RekeyTimeout = value
		case "RejectAfterTime":
			params.RejectAfterTime = value
		case "KeepaliveTimeout":
			params.KeepaliveTimeout = value
		case "MaxHandshakeAttempts":
			params.MaxHandshakeAttempts = value
		case "PersistentKeepalive":
			params.PersistentKeepalive = value
		}
	}

	// The knob strings above went in unchecked: api.Validate covers their
	// format and range along with everything else, and reporting a problem
	// once beats reporting it from two places in two wordings.
	problems = append(problems, params.Validate(mtu)...)
	return params, problems
}

func (f *Form) submit() {
	req, problems := f.build()
	if len(problems) > 0 {
		f.showErrors(problems)
		return
	}
	f.showErrors(nil)

	f.create.Disable()
	f.create.SetText(lang.L("Creating…"))

	go func() {
		server, err := f.env.Backend.CreateServer(req)

		fyne.Do(func() {
			f.create.Enable()
			f.create.SetText(lang.L("Create server"))
		})

		if err != nil {
			fyne.Do(func() { f.showErrors([]string{err.Error()}) })
			f.env.Notify.Fail(err)
			return
		}

		f.env.Notify.OK(lang.L("Server \"{{.Name}}\" created", map[string]any{"Name": server.Name}))
		fyne.Do(func() {
			f.name.SetText("")
			f.endpoint.SetText("")
			f.collapse()
		})
		f.env.Reload()
	}()
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
