// Package widgets holds the small building blocks every part of the page is
// assembled from: the button with the right cursor, the type scale helpers,
// chips, meters, the switch, the overflow menu and the cards. Nothing in here
// knows about servers or clients.
package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/testhook"
)

// ── Buttons ──────────────────────────────────────────────────────────────────

// Button is widget.Button with the hand cursor a browser user expects over
// anything clickable. Fyne's button deliberately reports the default arrow -
// correct for a desktop window, wrong for a page - and the wasm driver maps
// whatever a widget asks for onto the CSS cursor, so this is all it takes.
type Button struct {
	widget.Button
}

// NewButton is the constructor every button in this UI goes through, so none
// of them can be built without the cursor.
func NewButton(label string, icon fyne.Resource, tapped func()) *Button {
	b := &Button{}
	b.ExtendBaseWidget(b)
	b.Text = label
	b.Icon = icon
	b.OnTapped = tapped
	return b
}

func (b *Button) Cursor() desktop.Cursor {
	if b.Disabled() {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

// IconButton is a square, label-less button for a single glyph: the trailing
// action of a list row or a card header.
func IconButton(icon fyne.Resource, tapped func()) *Button {
	button := NewButton("", icon, tapped)
	button.Importance = widget.LowImportance
	return button
}

// ── Text fields ──────────────────────────────────────────────────────────────

// NewEntry is the single-line entry this page uses everywhere a value is
// short enough to fit its field.
//
// A stock widget.NewEntry() keeps an internal container.Scroll around its
// text, which makes it the innermost fyne.Scrollable under the pointer: the
// driver hands it every wheel event and it silently drops the ones it cannot
// use, so the page stops scrolling wherever the cursor happens to rest on an
// input. Turning that inner scroller off (which needs both fields - see
// entryRenderer.Layout in fyne) leaves no Scrollable in the way and the wheel
// reaches the page scroll again.
//
// The cost is that such an entry reports its whole text as its minimum width,
// so this is deliberately not used for the I1-I5 fields, whose values are
// megabyte-scale blobs.
func NewEntry() *widget.Entry {
	e := &widget.Entry{Wrapping: fyne.TextWrapOff, Scroll: fyne.ScrollNone}
	e.ExtendBaseWidget(e)
	return e
}

// EntryWithText is a prefilled single-line entry.
func EntryWithText(text string) *widget.Entry {
	entry := NewEntry()
	entry.SetText(text)
	return entry
}

// EntryWithPlaceholder is an empty single-line entry with a hint.
func EntryWithPlaceholder(placeholder string) *widget.Entry {
	entry := NewEntry()
	entry.SetPlaceHolder(placeholder)
	return entry
}

// NumberEntry is a prefilled field that still shows its accepted range once
// the value is cleared, which is exactly when the range is wanted.
func NumberEntry(text, limits string) *widget.Entry {
	entry := NewEntry()
	entry.SetPlaceHolder(limits)
	entry.SetText(text)
	return entry
}

// fieldMinWidth caps how wide a field is allowed to claim it must be.
//
// widget.Entry with its inner scroll turned off (see NewEntry) reports the
// width of its whole text, and of its placeholder, as its minimum. Two of the
// create form's fields carry a sentence for a placeholder, and one of them is
// three hundred points wide on its own - wider than a phone, once the card's
// padding is added. The field is clipped rather than the page being widened:
// the longest placeholder is a hint, not a value anyone has to read at once.
const fieldMinWidth float32 = 170

// clampWidth keeps a child from asking for more width than limit as its
// minimum, while still giving it every point the layout does hand it.
type clampWidth struct {
	limit float32
}

func (c *clampWidth) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
}

func (c *clampWidth) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var result fyne.Size
	for _, o := range objects {
		size := o.MinSize()
		result.Width = max(result.Width, min(size.Width, c.limit))
		result.Height = max(result.Height, size.Height)
	}
	return result
}

// ClampWidth caps the minimum width a block asks for; see fieldMinWidth.
func ClampWidth(limit float32, content fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&clampWidth{limit: limit}, content)
}

// AtLeast raises a block's minimum width to floor.
//
// The counterpart of ClampWidth, for content that is readable narrower but
// looks cramped: a form whose fields are clamped to a narrow minimum reports
// that minimum, and a dialog sizing itself to its content would shrink around
// it to nothing. A floor says how wide the form wants to be without forcing
// anything on a page too narrow to give it - the dialog clamps to the window.
func AtLeast(floor float32, content fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&atLeast{floor: floor}, content)
}

type atLeast struct {
	floor float32
}

func (a *atLeast) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
}

func (a *atLeast) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var min fyne.Size
	for _, o := range objects {
		size := o.MinSize()
		min.Width = max(min.Width, size.Width, a.floor)
		min.Height = max(min.Height, size.Height)
	}
	return min
}

// Labeled stacks a small caption above a field, the M3 text-field shape: the
// label sits on the page background and the fill belongs to the input alone.
func Labeled(caption string, field fyne.CanvasObject) fyne.CanvasObject {
	label := canvas.NewText(caption, style.Muted)
	label.TextSize = style.SizeLabel
	return container.NewVBox(container.NewPadded(label), ClampWidth(fieldMinWidth, field))
}

// Field is Labeled plus a hint line, the pair every form row is made of.
func Field(caption string, field fyne.CanvasObject, hint string) fyne.CanvasObject {
	rows := []fyne.CanvasObject{Labeled(caption, field)}
	if hint != "" {
		rows = append(rows, Hint(hint))
	}
	return container.NewVBox(rows...)
}

// ── Text ─────────────────────────────────────────────────────────────────────

// SectionTitle is the heading a block of the page opens with.
func SectionTitle(text string) fyne.CanvasObject {
	title := canvas.NewText(text, style.Text)
	title.TextSize = style.SizeTitle
	return title
}

// CardTitle is the name of a server or a client: one step below the section
// heading, still regular weight.
func CardTitle(text string) *canvas.Text {
	title := canvas.NewText(text, style.Text)
	title.TextSize = style.SizeHeading
	return title
}

// SmallText is the 12pt canvas text used for captions and counters.
func SmallText(text string, c color.Color) *canvas.Text {
	t := canvas.NewText(text, c)
	t.TextSize = style.SizeCaption
	return t
}

// MetaLabel is the second line of a list row: tabular figures, muted, and
// wrapped.
//
// Wrapped for the same reason Hint is: a canvas.Text reports the width of the
// whole string as its minimum. A server's meta line - port, IPv4 subnet, IPv6
// subnet, MTU - is about 60 characters of monospace, which is 430 points of
// minimum width: more than a phone has for the whole page, so the page would be
// laid out 430 points wide and clipped by the screen. A label measures its
// longest word instead and wraps the rest.
func MetaLabel(text string) fyne.CanvasObject {
	return NewParagraph(text, style.SizeCaption, style.Muted,
		fyne.TextStyle{Monospace: true}, 140)
}

// LabelText is the 13pt text of a chip or a list row's second line.
func LabelText(text string, c color.Color) *canvas.Text {
	t := canvas.NewText(text, c)
	t.TextSize = style.SizeLabel
	return t
}

// MonoText switches a text to tabular figures, for addresses, ports and byte
// counts, where digits of different widths make a column look drunk.
func MonoText(t *canvas.Text) *canvas.Text {
	t.TextStyle = fyne.TextStyle{Monospace: true}
	return t
}

// Hint is the muted explanation under a field.
//
// It wraps, and that is not a detail: a canvas.Text reports the width of its
// whole string as its minimum however it is laid out, and the longest hint in
// the create form - the IPv6 one - would then make the page demand four times
// the width of a phone. A wrapped label measures its longest word instead,
// which is what lets the form fold into one column.
func Hint(text string) fyne.CanvasObject {
	note := NewParagraph(text, style.SizeCaption, style.Muted,
		fyne.TextStyle{Italic: true}, 180)
	return container.NewPadded(note)
}

// SmallIcon is a 16px glyph in the muted colour, sized to sit next to
// SmallText on the same line.
func SmallIcon(icon fyne.Resource) fyne.CanvasObject {
	glyph := widget.NewIcon(theme.NewColoredResource(icon, theme.ColorNamePlaceHolder))
	box := container.NewGridWrap(fyne.NewSize(16, 16), glyph)
	// A couple of pixels on the right, so an icon sitting next to a number does
	// not read as part of it.
	return container.New(&insetLayout{h: 0, v: 0, right: 4}, box)
}

// MutedNote is an italic, word-wrapped explanation under a field or a grid.
func MutedNote(text string) fyne.CanvasObject {
	return NewParagraph(text, style.SizeLabel, style.Muted, fyne.TextStyle{Italic: true}, 220)
}

// ── Chips, badges, meters ────────────────────────────────────────────────────

// StatusChip is a chip tinted with a status colour and led by a dot of it -
// the running/stopped/active/suspended marker of a row.
func StatusChip(text string, c color.NRGBA) fyne.CanvasObject {
	dot := canvas.NewCircle(c)
	return pill(text, style.WithAlpha(c, 0x1f), c, container.NewGridWrap(fyne.NewSize(7, 7), dot))
}

// Badge is a small tinted tag: a client's I-settings, its scheduled
// suspension, a warning. Tonal, like the status chip, but without the dot.
func Badge(text string, c color.NRGBA) fyne.CanvasObject {
	return pill(text, style.WithAlpha(c, 0x1f), c, nil)
}

// Pill wraps any content in the rounded, filled shape a chip is.
func Pill(bg color.NRGBA, content fyne.CanvasObject) fyne.CanvasObject {
	background := canvas.NewRectangle(bg)
	background.CornerRadius = style.RadiusLG
	return container.NewStack(background, container.New(&chipInset{}, content))
}

// chipInset is Pill's padding. It is asymmetric on purpose: a canvas.Text box
// is ascent plus descent tall while the letters occupy its upper part, so an
// evenly padded box leaves the word sitting visibly high. These two numbers
// put the letters themselves in the middle at SizeLabel; they move with it.
const (
	chipPadTop    float32 = 7
	chipPadBottom float32 = 7
)

type chipInset struct{}

func (c *chipInset) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Move(fyne.NewPos(style.Space3, chipPadTop))
		o.Resize(fyne.NewSize(size.Width-style.Space3*2, size.Height-chipPadTop-chipPadBottom))
	}
}

func (c *chipInset) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var min fyne.Size
	for _, o := range objects {
		min = min.Max(o.MinSize())
	}
	return min.Add(fyne.NewSize(style.Space3*2, chipPadTop+chipPadBottom))
}

func pill(text string, bg color.NRGBA, fg color.NRGBA, leading fyne.CanvasObject) fyne.CanvasObject {
	label := canvas.NewText(text, fg)
	label.TextSize = style.SizeLabel

	row := []fyne.CanvasObject{}
	if leading != nil {
		row = append(row, container.NewCenter(leading))
	}
	row = append(row, container.NewCenter(label))
	return Pill(bg, container.NewHBox(row...))
}

// ── Switch ───────────────────────────────────────────────────────────────────

// Switch is the M3 switch: a rounded track and a thumb that slides across it.
// Fyne has no switch of its own, and the checkbox it does have reads as a form
// control rather than as the settings toggle this page is imitating.
type Switch struct {
	widget.BaseWidget

	checked   bool
	onChanged func(bool)
}

// NewSwitch builds a switch in the given state; onChanged runs on every tap.
func NewSwitch(checked bool, onChanged func(bool)) *Switch {
	s := &Switch{checked: checked, onChanged: onChanged}
	s.ExtendBaseWidget(s)
	return s
}

// Checked reports the current state.
func (s *Switch) Checked() bool { return s.checked }

// SetChecked moves the switch without calling onChanged, for code that is
// putting the widget back into a known state rather than reacting to the user.
func (s *Switch) SetChecked(checked bool) {
	if s.checked == checked {
		return
	}
	s.checked = checked
	s.Refresh()
}

// Tapped is a left click on the whole switch.
func (s *Switch) Tapped(*fyne.PointEvent) {
	s.checked = !s.checked
	s.Refresh()
	if s.onChanged != nil {
		s.onChanged(s.checked)
	}
}

// Cursor is the hand a browser user expects over anything clickable.
func (s *Switch) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (s *Switch) CreateRenderer() fyne.WidgetRenderer {
	r := &switchRenderer{s: s}
	r.track = canvas.NewRectangle(style.SurfaceHigh)
	r.track.CornerRadius = 16
	r.track.StrokeColor = style.Border
	r.track.StrokeWidth = 2
	r.thumb = canvas.NewRectangle(style.Muted)
	r.thumb.CornerRadius = 10
	r.objects = []fyne.CanvasObject{r.track, r.thumb}
	r.Refresh()
	return r
}

type switchRenderer struct {
	s       *Switch
	track   *canvas.Rectangle
	thumb   *canvas.Rectangle
	objects []fyne.CanvasObject
}

func (r *switchRenderer) Layout(size fyne.Size) {
	r.track.Resize(size)
	r.track.Move(fyne.NewPos(0, 0))
	const inset float32 = 5
	thumbSize := size.Height - inset*2
	x := inset
	if r.s.checked {
		x = size.Width - inset - thumbSize
	}
	r.thumb.Resize(fyne.NewSize(thumbSize, thumbSize))
	r.thumb.Move(fyne.NewPos(x, inset))
}

func (r *switchRenderer) MinSize() fyne.Size { return fyne.NewSize(48, 28) }

func (r *switchRenderer) Refresh() {
	if r.s.checked {
		r.track.FillColor = style.Primary
		r.track.StrokeColor = style.Primary
		r.thumb.FillColor = style.OnPrimary
	} else {
		r.track.FillColor = style.SurfaceHigh
		r.track.StrokeColor = style.Border
		r.thumb.FillColor = style.Muted
	}
	r.track.Refresh()
	r.thumb.Refresh()
	r.Layout(r.s.Size())
}

func (r *switchRenderer) Destroy()                     {}
func (r *switchRenderer) Objects() []fyne.CanvasObject { return r.objects }

// ToggleRow is a setting with its M3 switch: the label on the left, the switch
// on the right edge of the row. The label wraps rather than pushing the switch
// off the row on a narrow page.
func ToggleRow(label string, sw *Switch) fyne.CanvasObject {
	text := NewParagraph(label, style.SizeBody, style.Text, fyne.TextStyle{}, 140)
	// The label takes the whole row and wraps; only the switch is pinned to
	// the right edge, which is what keeps the setting's name at the start of
	// the line the way M3 lists it.
	return container.NewBorder(nil, nil, nil, container.NewCenter(sw), text)
}

// ── Menu ─────────────────────────────────────────────────────────────────────

// MenuItem is one entry of an overflow menu. ID names the entry for the
// browser suite, which cannot click an item of a menu it has not opened.
type MenuItem struct {
	ID    string
	Label string
	Icon  fyne.Resource
	Do    func()
}

// MenuButton is the trailing "more" button of a row: a single glyph that opens
// a pop-up menu under itself. Every action that used to be its own button in
// the row lives in here, which is what keeps a row readable.
func MenuButton(prefix string, items []MenuItem) *Button {
	registerMenuItems(prefix, items)
	button := NewButton("", theme.MoreHorizontalIcon(), nil)
	button.Importance = widget.LowImportance
	button.OnTapped = func() { showMenu(button, prefix, items) }
	return button
}

// registerMenuItems publishes a menu's entries under stable names while the
// menu is being built: the suite picks one by id and runs it directly, instead
// of opening the pop-up and guessing where an entry landed.
func registerMenuItems(prefix string, items []MenuItem) {
	if prefix == "" {
		return
	}
	for _, item := range items {
		if item.ID == "" || item.Do == nil {
			continue
		}
		testhook.RegisterAction(prefix+"."+item.ID, item.Do)
	}
}

func showMenu(from fyne.CanvasObject, prefix string, items []MenuItem) {
	canvasFor := fyne.CurrentApp().Driver().CanvasForObject(from)
	if canvasFor == nil {
		return
	}
	showMenuAt(from, items, canvasFor)
}

// ── Surfaces ─────────────────────────────────────────────────────────────────

// Card wraps content in the page's standard surface: the M3 filled card, a
// tonal step above the background and a large radius, with no border.
func Card(content fyne.CanvasObject) fyne.CanvasObject {
	return surface(content, style.Surface, style.RadiusXL)
}

// HeroCard is the accent-filled card the dashboard's headline figure lives in.
func HeroCard(content fyne.CanvasObject) fyne.CanvasObject {
	return surface(content, style.PrimaryContainer, style.RadiusXL)
}

// InnerCard is the tonal step inside a card: a block that belongs to the card
// rather than to the page.
func InnerCard(content fyne.CanvasObject) fyne.CanvasObject {
	return surface(content, style.SurfaceAlt, style.RadiusLG)
}

func surface(content fyne.CanvasObject, fill color.NRGBA, radius float32) fyne.CanvasObject {
	bg := canvas.NewRectangle(fill)
	bg.CornerRadius = radius
	// 20/16, not the 4px the theme pads with: a card that hugs its text looks
	// like a table cell, and a label sitting on the edge reads as clipped.
	return container.NewStack(bg, container.New(&insetLayout{h: style.Space5 - style.Space1, v: style.Space4}, content))
}

// Icon is a glyph at an explicit size, drawn in the page's own text colour
// rather than the muted placeholder one: inside a tonal disc it has to read as
// the thing the row is about.
func Icon(icon fyne.Resource, size float32) fyne.CanvasObject {
	glyph := widget.NewIcon(theme.NewColoredResource(icon, theme.ColorNameForeground))
	return container.NewGridWrap(fyne.NewSize(size, size), glyph)
}

// Spacer is a vertical gap of an exact height, for the blocks of a card whose
// rhythm the theme padding does not describe.
func Spacer(height float32) fyne.CanvasObject {
	blank := canvas.NewRectangle(color.Transparent)
	blank.SetMinSize(fyne.NewSize(1, height))
	return blank
}

// Gap is fixed empty space between two objects of a row.
func Gap(width float32) fyne.CanvasObject {
	blank := canvas.NewRectangle(color.Transparent)
	blank.SetMinSize(fyne.NewSize(width, 1))
	return blank
}

// Inset pads an object by an explicit number of points. A canvas.Text draws
// from the top of its box, so text that has to sit inside a row of known height
// needs slack above it rather than a centre layout of exactly that height.
func Inset(content fyne.CanvasObject, h, v float32) fyne.CanvasObject {
	return container.New(&insetLayout{h: h, v: v}, content)
}

// Separator is the hairline between blocks of one card. M3 separates by space;
// this stays for the places where a list runs long.
func Separator() fyne.CanvasObject {
	line := canvas.NewRectangle(style.Border)
	line.SetMinSize(fyne.NewSize(0, 1))
	return line
}

// InfoGrid lays key/value pairs out in two columns, values in monospace.
func InfoGrid(rows [][2]string) fyne.CanvasObject {
	grid := container.NewGridWithColumns(2)
	for _, row := range rows {
		key := LabelText(row[0], style.Muted)
		value := MonoText(LabelText(row[1], style.Text))
		grid.Add(container.NewHBox(key, value))
	}
	return grid
}

// MonospaceView is a scrollable text box for configs, plus the setter that
// replaces its contents. Entry has no read-only mode that still allows
// selecting and copying, so edits are reverted - which means the widget needs
// to know which text is the legitimate one at any moment, and callers must go
// through the setter rather than SetText.
func MonospaceView(text string) (*widget.Entry, func(string)) {
	view := widget.NewMultiLineEntry()
	view.TextStyle = fyne.TextStyle{Monospace: true}
	view.Wrapping = fyne.TextWrapBreak

	shown := text
	set := func(next string) {
		shown = next
		view.SetText(next)
	}

	view.OnChanged = func(typed string) {
		if typed != shown {
			view.SetText(shown)
		}
	}

	set(text)
	return view, set
}

// Truncate cuts a value down to limit bytes with an ellipsis, for the places
// where a megabyte-scale blob has to fit on one line.
func Truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

// Uptime renders a duration in seconds the way an operator reads it: the
// two largest units that matter - "3d 4h", "4h 12m", "12m" - and "< 1m"
// while it is still counting its first minute.
func Uptime(seconds float64) string {
	total := int(seconds)
	days, hours, minutes := total/86400, total%86400/3600, total%3600/60
	switch {
	case days > 0:
		return lang.L("{{.Count}}d", map[string]any{"Count": days}) + " " +
			lang.L("{{.Count}}h", map[string]any{"Count": hours})
	case hours > 0:
		return lang.L("{{.Count}}h", map[string]any{"Count": hours}) + " " +
			lang.L("{{.Count}}m", map[string]any{"Count": minutes})
	case minutes > 0:
		return lang.L("{{.Count}}m", map[string]any{"Count": minutes})
	default:
		return "< " + lang.L("{{.Count}}m", map[string]any{"Count": 1})
	}
}

// insetLayout pads its children by an explicit number of pixels, which the
// theme padding cannot express per-side (used by the small chips).
type insetLayout struct {
	h, v  float32
	right float32
}

func (i *insetLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Move(fyne.NewPos(i.h, i.v))
		o.Resize(size.Subtract(fyne.NewSize(i.h*2+i.right, i.v*2)))
	}
}

func (i *insetLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var min fyne.Size
	for _, o := range objects {
		min = min.Max(o.MinSize())
	}
	return min.Add(fyne.NewSize(i.h*2+i.right, i.v*2))
}
