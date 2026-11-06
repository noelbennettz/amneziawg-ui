// Package style is the single palette the page is drawn in, plus the shape and
// type scale every component measures itself against, and the fyne theme that
// applies the same values to the stock widgets.
//
// The look is Material 3 (dark): neutral near-black surfaces, one blue accent,
// large corner radii, no borders - separation comes from tonal steps and space,
// not from hairlines - and a type scale of regular-weight sizes rather than
// bold everywhere. Every component colours its own canvas objects from the
// exported values, so a shade is changed in one place.
package style

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The M3 dark palette, blue accent. The UI ships one palette, so every colour
// below is returned regardless of the variant the toolkit asks for.
var (
	Background  = rgb(0x131314) // page
	Surface     = rgb(0x1e1f20) // cards
	SurfaceAlt  = rgb(0x2a2b2d) // inputs, chips, secondary buttons
	SurfaceHigh = rgb(0x333537) // menus, snackbar, floating layers
	Border      = rgb(0x3c4043) // the rare hairline that is still wanted

	Text  = rgb(0xe3e3e3) // on-surface
	Muted = rgb(0xc4c7c5) // on-surface-variant

	// The accent, in the tones M3 uses it for: the pastel one that reads on a
	// dark surface, and the deeper container pair the hero card is drawn in.
	Primary            = rgb(0xa8c7fa)
	OnPrimary          = rgb(0x0a305f)
	PrimaryContainer   = rgb(0x0842a0)
	OnPrimaryContainer = rgb(0xd3e3fd)

	// Status colours are pastel in the M3 dark scheme rather than saturated.
	Success = rgb(0xb7f0a8)
	Warning = rgb(0xf7dca6)
	Error   = rgb(0xffb4ab)
	Accent  = rgb(0xd3bbff)

	hover       = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x14}
	pressed     = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x22}
	focus       = color.NRGBA{R: 0xa8, G: 0xc7, B: 0xfa, A: 0x33}
	selection   = color.NRGBA{R: 0xa8, G: 0xc7, B: 0xfa, A: 0x55}
	scrollBar   = color.NRGBA{R: 0x4a, G: 0x4c, B: 0x50, A: 0xcc}
	shadow      = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x66}
	disabled    = color.NRGBA{R: 0x6b, G: 0x6e, B: 0x72, A: 0xff}
	disabledBtn = color.NRGBA{R: 0x24, G: 0x25, B: 0x26, A: 0xff}
)

// Corner radii, in points. XL is what cards and dialogs use, LG the inner
// blocks and the server/client rows, MD the text fields and menus.
const (
	RadiusXL float32 = 28
	RadiusLG float32 = 24
	RadiusMD float32 = 16
	RadiusSM float32 = 12
)

// The spacing scale, in points.
const (
	Space1 float32 = 4
	Space2 float32 = 8
	Space3 float32 = 12
	Space4 float32 = 16
	Space5 float32 = 24
	Space6 float32 = 32
)

// Breakpoint is the page width, in points, below which the layout stacks: the
// app bar puts its facts under the title, the dashboard puts the gauges under
// the chart card, a row puts its actions under its name.
//
// It is not an aesthetic choice: the two-column layout needs about 950 points
// of its own - a bar carrying four pills next to a 28pt title, a chart card
// beside four gauges, and a card row with its actions at the right edge. A
// window narrower than that would draw the wide layout and clip the right-hand
// side of every card, which is exactly what it did at 720. 1024 leaves the
// summary pill room to grow a few characters before it pushes the bar past the
// window again.
const Breakpoint float32 = 1024

// The type scale, in points. Titles are regular weight: M3 builds hierarchy
// out of size and space, not out of bolding everything.
const (
	SizeDisplay float32 = 28
	SizeTitle   float32 = 22
	SizeHeading float32 = 20
	SizeBody    float32 = 15
	SizeLabel   float32 = 13
	SizeCaption float32 = 12
)

// rgb builds an opaque NRGBA from its RGB hex.
func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 0xff}
}

// WithAlpha returns the colour with its opacity replaced.
func WithAlpha(c color.NRGBA, alpha uint8) color.NRGBA {
	c.A = alpha
	return c
}

// darkTheme wraps the default theme and forces the dark variant everywhere, so
// the app looks the same no matter what the browser reports for
// prefers-color-scheme.
type darkTheme struct {
	fyne.Theme
}

// NewTheme returns the page's theme.
func NewTheme() fyne.Theme {
	return &darkTheme{Theme: theme.DefaultTheme()}
}

func (t *darkTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return Background
	case theme.ColorNameButton:
		return SurfaceAlt
	case theme.ColorNameDisabledButton:
		return disabledBtn
	case theme.ColorNameDisabled:
		return disabled
	case theme.ColorNameForeground:
		return Text
	case theme.ColorNameForegroundOnPrimary:
		return OnPrimary
	case theme.ColorNameForegroundOnError:
		return rgb(0x410002)
	case theme.ColorNameForegroundOnSuccess:
		return rgb(0x00320a)
	case theme.ColorNameForegroundOnWarning:
		return rgb(0x2a1c00)
	case theme.ColorNameHeaderBackground:
		return Surface
	case theme.ColorNameHover:
		return hover
	case theme.ColorNameFocus:
		return focus
	case theme.ColorNameHyperlink:
		return Primary
	case theme.ColorNameInputBackground:
		// One step above the dialog or the card the field sits on, which is
		// what makes a filled M3 field read as a field rather than as a hole.
		return SurfaceHigh
	case theme.ColorNameInputBorder:
		// An M3 text field is a filled surface, not a box, so the stroke the
		// entry draws has nothing to show - see SizeNameInputBorder below for
		// why it cannot simply be zero.
		return color.Transparent
	case theme.ColorNameMenuBackground:
		return SurfaceHigh
	case theme.ColorNameOverlayBackground:
		// The popup renders this behind its content; transparent leaves just
		// the shadow, so the card a dialog or a menu draws is the only surface
		// and no rim of a second one shows around its rounded corners.
		return color.Transparent
	case theme.ColorNamePlaceHolder:
		return rgb(0x8b8f93)
	case theme.ColorNamePressed:
		return pressed
	case theme.ColorNamePrimary:
		return Primary
	case theme.ColorNameScrollBar:
		return scrollBar
	case theme.ColorNameScrollBarBackground:
		return Background
	case theme.ColorNameSelection:
		return selection
	case theme.ColorNameSeparator:
		return Border
	case theme.ColorNameShadow:
		return shadow
	case theme.ColorNameSuccess:
		return Success
	case theme.ColorNameWarning:
		return Warning
	case theme.ColorNameError:
		return Error
	}

	return t.Theme.Color(name, theme.VariantDark)
}

func (t *darkTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameInputRadius:
		return RadiusMD
	case theme.SizeNameInputBorder:
		// fyne builds the text caret out of this size: entryContentRenderer.
		// moveCursor resizes the caret rectangle to `SizeNameInputBorder` by
		// the line height, so a theme that answers 0 - which is how the
		// outline is removed from a filled field - removes the caret with it.
		// The field took focus, swallowed the keystrokes and showed nothing at
		// all: no cursor, no outline, no change of colour. Two points is the
		// caret M3 draws.
		//
		// The stroke that the same number now sizes is drawn in
		// ColorNameInputBorder, which is transparent, so the field still looks
		// like a filled surface; what it costs is the two points of inset the
		// entry takes inside its own bounds.
		return 2
	case theme.SizeNameButtonRadius:
		// Half of a stock button's height: the pill M3 buttons are drawn as.
		return 20
	case theme.SizeNameSelectionRadius:
		return RadiusSM
	case theme.SizeNamePopupRadius:
		return RadiusXL
	case theme.SizeNameScrollBar:
		return 10
	case theme.SizeNameCaptionText:
		// The type scale's caption step, for the widgets that ask the theme
		// for a size by name rather than drawing their own text - a wrapped
		// label, above all, which is what a long hint has to be.
		return SizeCaption
	}

	return t.Theme.Size(name)
}
