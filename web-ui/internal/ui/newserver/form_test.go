package newserver

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"fyne.io/fyne/v2/widget"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/env"
	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// pageState is a State with whatever the test needs the page to have heard
// from the backend.
type pageState struct {
	mtu           int
	subnets       []string
	subnetsV6     []string
	port          int
	defaultSubnet string
	dns           string
	// noAutoStart is AUTO_START_SERVERS=false: a new server is created
	// stopped rather than started.
	noAutoStart bool
}

func (s pageState) ServerMTU() int {
	if s.mtu < api.MinMTU || s.mtu > api.MaxMTU {
		return DefaultMTU
	}
	return s.mtu
}

func (s pageState) DefaultPort() int {
	if s.port == 0 {
		return DefaultPort
	}
	return s.port
}

func (s pageState) DefaultSubnet() string {
	if s.defaultSubnet == "" {
		return DefaultSubnet
	}
	return s.defaultSubnet
}

func (s pageState) DefaultDNS() string {
	if s.dns == "" {
		return DefaultDNS
	}
	return s.dns
}

func (s pageState) DefaultAutoStart() bool { return !s.noAutoStart }

func (s pageState) TakenPorts() map[int]string { return map[int]string{} }

func (s pageState) TakenSubnets() map[string]string {
	taken := map[string]string{}
	for _, subnet := range s.subnets {
		taken[subnet] = `server "srv"`
	}
	return taken
}

func (s pageState) TakenSubnetsV6() map[string]string {
	taken := map[string]string{}
	for _, subnet := range s.subnetsV6 {
		taken[subnet] = `server "srv"`
	}
	return taken
}

// The operator sets DEFAULT_MTU on the backend; the simple mode has no MTU
// field to show it in, so it has to be what a server created there actually
// gets. It used to send a number of its own and the setting was ignored.
func TestSimpleModeFollowsTheBackendMTU(t *testing.T) {
	f := &Form{state: pageState{mtu: 1280}}

	req := f.generated("srv", 54844)
	if req.MTU != 1280 {
		t.Fatalf("MTU = %d, want the backend's 1280", req.MTU)
	}
	if problems := req.ObfuscationParams.Validate(req.MTU); len(problems) > 0 {
		t.Fatalf("payload does not validate at that MTU:\n%s", strings.Join(problems, "\n"))
	}
	// The wider MTU headroom is the point: at 1280 a padding can use the whole
	// recommended window instead of collapsing onto its floor.
	if wire := req.MTU + 60 + req.ObfuscationParams.S4; wire > 1500 {
		t.Fatalf("a full transport packet would be %d bytes on the wire", wire)
	}
}

// A backend that reports nothing usable - the status call has not landed yet,
// or DEFAULT_MTU is outside the range a server may use - leaves the built-in
// default in place rather than sending something invalid.
func TestSimpleModeFallsBackToTheBuiltInMTU(t *testing.T) {
	for _, reported := range []int{0, 1279, 1441, -1} {
		f := &Form{state: pageState{mtu: reported}}

		if got := f.generated("srv", 54844).MTU; got != DefaultMTU {
			t.Errorf("backend reported %d: MTU = %d, want the built-in %d", reported, got, DefaultMTU)
		}
	}
}

// generated() is the whole simple-mode payload: a name, a port, and every
// other field derived. It is what "Create server" sends with "Advanced
// settings" unticked.
func TestSimpleModePayloadIsValid(t *testing.T) {
	f := &Form{state: pageState{}}

	req := f.generated("My VPN Server", 54844)

	if req.Name != "My VPN Server" || req.Port != 54844 {
		t.Fatalf("the two fields the user filled in should survive: %+v", req)
	}
	if req.Subnet != "10.0.0.0/24" {
		t.Errorf("Subnet = %q, want the first free /24", req.Subnet)
	}
	if req.MTU != DefaultMTU {
		t.Errorf("MTU = %d, want %d", req.MTU, DefaultMTU)
	}
	if req.DNS != DefaultDNS {
		t.Errorf("DNS = %v, want %q", req.DNS, DefaultDNS)
	}
	if req.Endpoint != "" {
		t.Errorf("Endpoint = %q, want it left to the backend's detected public IP", req.Endpoint)
	}
	if req.Obfuscation == nil || !*req.Obfuscation || req.AutoStart == nil || !*req.AutoStart {
		t.Fatalf("the simple mode should obfuscate and auto-start: %+v", req)
	}
	if req.ObfuscationParams == nil {
		t.Fatal("the simple mode should carry a generated parameter set")
	}
	if problems := req.ObfuscationParams.Validate(req.MTU); len(problems) > 0 {
		t.Fatalf("the generated payload does not pass validation:\n%s", strings.Join(problems, "\n"))
	}
}

// A second server created in the simple mode must not land on the subnet the
// first one took, since neither the form nor the backend would notice.
func TestSimpleModePicksAFreeSubnet(t *testing.T) {
	f := &Form{state: pageState{subnets: []string{"10.0.0.0/24", "10.1.0.0/24"}}}

	// The neighbour of the default rather than the next second octet: with
	// 10.0.0.0/24 taken the search moves one /24 along, which keeps a pair of
	// servers inside one block instead of scattering them.
	if got := f.generated("third", 54846).Subnet; got != "10.0.1.0/24" {
		t.Errorf("Subnet = %q, want 10.0.1.0/24", got)
	}
}

// An existing server on a wider block covers every /24 inside it, so the
// simple mode has to look past it rather than match subnets as strings.
func TestSimpleModeSkipsSubnetsInsideAWiderOne(t *testing.T) {
	f := &Form{state: pageState{subnets: []string{"10.0.0.0/15"}}}

	if got := f.generated("second", 54845).Subnet; got != "10.2.0.0/24" {
		t.Errorf("Subnet = %q, want 10.2.0.0/24", got)
	}
}

// The operator's DEFAULT_SUBNET is what the first server gets, even though the
// simple-mode search would otherwise start at 10.0.0.0/24.
func TestSimpleModePrefersTheOperatorsSubnet(t *testing.T) {
	f := &Form{state: pageState{defaultSubnet: "192.168.50.0/24"}}

	if got := f.generated("first", 54844).Subnet; got != "192.168.50.0/24" {
		t.Errorf("Subnet = %q, want the operator's 192.168.50.0/24", got)
	}
}

// The simple mode sends DEFAULT_DNS: the DNS field is not on screen there, so
// the backend's setting is what a server created in that mode gets.
func TestSimpleModeFollowsTheBackendDNS(t *testing.T) {
	f := &Form{state: pageState{dns: "9.9.9.9,149.112.112.112"}}

	if got := f.generated("srv", 54844).DNS; got != "9.9.9.9,149.112.112.112" {
		t.Errorf("DNS = %q, want the backend's 9.9.9.9,149.112.112.112", got)
	}
}

// The simple mode used to send auto_start: true whatever AUTO_START_SERVERS
// said, so an operator who turned starting off got every server created from
// the panel started anyway. The panel's own path - the switch in the advanced
// mode - is seeded from the same setting, and only over a switch it set itself.
func TestAutoStartFollowsTheBackendSetting(t *testing.T) {
	for _, off := range []bool{false, true} {
		f := &Form{state: pageState{noAutoStart: off}}

		if got, want := *f.generated("srv", 54844).AutoStart, !off; got != want {
			t.Errorf("AUTO_START_SERVERS=%v: simple mode sends auto_start=%v, want %v", !off, got, want)
		}
	}

	test.NewApp()
	f := New(&env.Env{}, pageState{noAutoStart: true}, func() {})
	if f.autoStart.Checked() {
		t.Error("the switch starts on although the backend says not to start new servers")
	}

	// A switch the operator flipped is theirs: a later status call does not
	// put it back.
	f.autoStart.SetChecked(true)
	f.state = pageState{noAutoStart: true}
	f.SeedDefaults()
	if !f.autoStart.Checked() {
		t.Error("seeding undid a switch the operator set")
	}
}

// SeedDefaults is what puts DEFAULT_PORT, DEFAULT_SUBNET and DEFAULT_DNS into
// the fields once /api/system/status has answered - but only into fields the
// form itself filled in: a value the operator typed over stays.
func TestSeedDefaultsKeepsWhatTheUserTyped(t *testing.T) {
	test.NewApp()

	state := pageState{port: 51820, defaultSubnet: "192.168.50.0/24", dns: "9.9.9.9"}
	f := New(&env.Env{}, state, func() {})

	f.SeedDefaults()
	for _, want := range []struct {
		field string
		entry *widget.Entry
	}{
		{"port", f.port}, {"subnet", f.subnet}, {"dns", f.dns},
	} {
		if want.entry.Text == "" {
			t.Errorf("%s is empty after seeding", want.field)
		}
	}
	if f.port.Text != "51820" {
		t.Errorf("port = %q, want the backend's 51820", f.port.Text)
	}
	if f.subnet.Text != "192.168.50.0/24" {
		t.Errorf("subnet = %q, want the backend's 192.168.50.0/24", f.subnet.Text)
	}
	if f.dns.Text != "9.9.9.9" {
		t.Errorf("dns = %q, want the backend's 9.9.9.9", f.dns.Text)
	}

	// What the operator typed is theirs: a later status call must not undo it.
	f.port.SetText("41000")
	f.subnet.SetText("10.99.0.0/24")
	f.dns.SetText("1.1.1.1")
	f.SeedDefaults()

	if f.port.Text != "41000" || f.subnet.Text != "10.99.0.0/24" || f.dns.Text != "1.1.1.1" {
		t.Errorf("typed values were overwritten: %q %q %q", f.port.Text, f.subnet.Text, f.dns.Text)
	}
}

// The notes of the create form must never be drawn on top of each other, at
// whichever width the page is laid out - including the pass that narrows it,
// where a note wraps into more lines than the height the layout gave it before
// it knew the width.
//
// Every paragraph is walked rather than a named pair: the form holds a dozen
// of them (the hints under the fields, the note over the randomise button, the
// explanations of the parameter block), and the bug this guards against hit
// whichever of them the width made taller.
func TestCreateFormGivesItsNotesTheRoomTheyNeed(t *testing.T) {
	test.NewApp()

	f := New(&env.Env{}, pageState{}, func() {})
	f.toggleOpen()
	f.advancedMode.Tapped(nil) // advanced mode, and the obfuscation block with it

	page := widgets.Rewrapped(container.NewVBox(f.CanvasObject()))
	w := test.NewWindow(page)
	defer w.Close()

	for _, width := range []float32{1320, 700, 1320} {
		f.SetNarrow(width < style.Breakpoint)
		w.Resize(fyne.NewSize(width, 2400))

		eachParagraph(page, func(p *widgets.Paragraph) {
			if need := p.MinSize().Height; p.Size().Height+0.5 < need {
				t.Errorf("at %.0f wide: the note at y=%.0f draws %.0f points of text in the %.0f it was given",
					width, p.Position().Y, need, p.Size().Height)
			}
		})
	}
}

// eachParagraph visits every Paragraph in a tree, stepping into containers and
// into the objects the widgets around them draw.
func eachParagraph(obj fyne.CanvasObject, visit func(*widgets.Paragraph)) {
	switch node := obj.(type) {
	case *widgets.Paragraph:
		visit(node)
	case *fyne.Container:
		for _, child := range node.Objects {
			eachParagraph(child, visit)
		}
	case fyne.Widget:
		for _, child := range test.WidgetRenderer(node).Objects() {
			eachParagraph(child, visit)
		}
	}
}

// A second server stays inside the block the operator chose. DEFAULT_SUBNET is
// the first answer while it is free; after that the neighbour is the next /24
// of the same first two octets, not the built-in 10.0.0.0/24 - which is what a
// server created after the first one used to get, and what made the setting
// look like it had been ignored.
func TestSimpleModeStaysInTheOperatorsBlock(t *testing.T) {
	cases := []struct {
		defaultSubnet string
		taken         []string
		want          string
	}{
		{"10.9.0.0/24", []string{"10.9.0.0/24"}, "10.9.1.0/24"},
		{"10.9.0.0/24", []string{"10.9.0.0/24", "10.9.1.0/24"}, "10.9.2.0/24"},
		{"192.168.50.0/24", []string{"192.168.50.0/24"}, "192.168.51.0/24"},
		// A /16 has no /24 siblings to move to without splitting the block,
		// so the search falls back to the 10.x block rather than inventing
		// one inside the operator's range.
		{"10.9.0.0/16", []string{"10.9.0.0/16"}, "10.0.0.0/24"},
	}

	for _, c := range cases {
		f := &Form{state: pageState{defaultSubnet: c.defaultSubnet, subnets: c.taken}}
		if got := f.nextFreeSubnet(); got != c.want {
			t.Errorf("DEFAULT_SUBNET=%s taken=%v: subnet = %q, want %q", c.defaultSubnet, c.taken, got, c.want)
		}
	}
}

// liveState is a page whose backend answers change while the form is open: the
// status call lands, servers are created, the operator edits the compose file.
// pageState cannot do that - it is a value the form copies at build time.
type liveState struct {
	port     int
	subnet   string
	dns      string
	taken    map[int]string
	takenNet map[string]string
}

func (s *liveState) ServerMTU() int                    { return DefaultMTU }
func (s *liveState) TakenPorts() map[int]string        { return s.taken }
func (s *liveState) TakenSubnets() map[string]string   { return s.takenNet }
func (s *liveState) TakenSubnetsV6() map[string]string { return map[string]string{} }
func (s *liveState) DefaultAutoStart() bool            { return true }

func (s *liveState) DefaultPort() int {
	if s.port == 0 {
		return DefaultPort
	}
	return s.port
}

func (s *liveState) DefaultSubnet() string {
	if s.subnet == "" {
		return DefaultSubnet
	}
	return s.subnet
}

func (s *liveState) DefaultDNS() string {
	if s.dns == "" {
		return DefaultDNS
	}
	return s.dns
}

// The form has to keep following DEFAULT_PORT and DEFAULT_SUBNET for as long
// as the page is open, including after it moved the fields itself to avoid a
// collision. It used to write those auto-picked values without recording them
// as its own, so a later status call read them as something the operator had
// typed and left them alone - which is what made a changed DEFAULT_PORT
// disappear as soon as one server existed.
func TestTheFormKeepsFollowingTheBackendDefaults(t *testing.T) {
	test.NewApp()

	state := &liveState{taken: map[int]string{}, takenNet: map[string]string{}}
	f := New(&env.Env{}, state, func() {})

	// The first status call lands: the operator configured 51998 and
	// 10.9.0.0/24.
	state.port, state.subnet = 51998, "10.9.0.0/24"
	f.SeedDefaults()
	if f.port.Text != "51998" || f.subnet.Text != "10.9.0.0/24" {
		t.Fatalf("after the first status call: port %q subnet %q", f.port.Text, f.subnet.Text)
	}

	// A server takes both, and the form picks the next free ones when it is
	// opened again.
	state.taken[51998] = `server "first"`
	state.takenNet["10.9.0.0/24"] = `server "first"`
	f.toggleOpen()
	if f.port.Text != "51999" || f.subnet.Text != "10.9.1.0/24" {
		t.Fatalf("after the collision: port %q subnet %q, want 51999 and 10.9.1.0/24", f.port.Text, f.subnet.Text)
	}

	// The operator edits the compose file and restarts the container; the
	// status poll is what carries the new values into the open page.
	state.port, state.subnet = 52010, "10.20.0.0/24"
	f.SeedDefaults()
	if f.port.Text != "52010" {
		t.Errorf("port = %q, want the new DEFAULT_PORT 52010", f.port.Text)
	}
	if f.subnet.Text != "10.20.0.0/24" {
		t.Errorf("subnet = %q, want the new DEFAULT_SUBNET 10.20.0.0/24", f.subnet.Text)
	}

	// And a value the operator actually typed is still theirs.
	f.port.SetText("41000")
	f.subnet.SetText("10.99.0.0/24")
	state.port, state.subnet = 52011, "10.21.0.0/24"
	f.SeedDefaults()
	if f.port.Text != "41000" || f.subnet.Text != "10.99.0.0/24" {
		t.Errorf("typed values were overwritten: %q %q", f.port.Text, f.subnet.Text)
	}
}
