package widgets

import (
	"image/color"
	"strings"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Paragraph is text that wraps to the width it is given.
//
// It exists because neither of fyne's two text widgets can be used for this:
// a canvas.Text reports the width of its whole string as its minimum, so a
// sentence makes the page demand more width than a phone has, and
// widget.Label with wrapping turned on reports its minimum as the *height at
// its minimum width* - one wrapped line per word, a few hundred points of
// height for a row's second line. So the wrapping is done here, where the
// width that matters is known: Layout reports the width the text is allowed to
// be as narrow as, and the lines are cut to whatever width the layout hands
// over.
type Paragraph struct {
	widget.BaseWidget

	full  string
	size  float32
	style fyne.TextStyle
	color color.Color

	// minWidth is how narrow the block may get: below it the text is left to
	// overflow rather than made taller than the minimum already reported.
	minWidth float32

	lines []string
	box   *fyne.Container
}

// NewParagraph builds wrapped text with the given type size and colour, no
// narrower than minWidth.
func NewParagraph(text string, size float32, c color.Color, style fyne.TextStyle, minWidth float32) *Paragraph {
	p := &Paragraph{full: text, size: size, style: style, color: c, minWidth: minWidth, lines: []string{text}}
	p.ExtendBaseWidget(p)
	p.box = container.NewVBox()
	p.apply(p.wrap(minWidth))
	return p
}

func (p *Paragraph) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.box)
}

// Resize re-wraps to the width actually given.
func (p *Paragraph) Resize(size fyne.Size) {
	width := size.Width
	if width < p.minWidth {
		width = p.minWidth
	}
	p.BaseWidget.Resize(size)

	if p.apply(p.wrap(width)) {
		p.box.Resize(size)
	}
}

// MinSize is the height of the lines it is currently showing, at the narrowest
// width it accepts.
//
// A wrapped paragraph's height depends on the width it is given, which MinSize
// cannot know: reporting the height it would take at minWidth leaves a block of
// empty space under it in a wide column (four lines' worth, in a card header
// whose meta is one line). The first pass reports the narrow figure, which is
// safe; every Resize narrows it to the truth, and the parent re-measures on the
// next layout pass.
func (p *Paragraph) MinSize() fyne.Size {
	lines := p.lines
	if len(lines) == 0 {
		lines = p.wrap(p.minWidth)
	}
	return fyne.NewSize(p.minWidth, float32(len(lines))*lineHeight(p.size, p.style))
}

// apply shows the given lines, and reports whether that changed anything. The
// text is drawn as one canvas.Text per line: a canvas.Text is a single line,
// and a newline inside one is not a break but a glyph.
func (p *Paragraph) apply(lines []string) bool {
	if p.box == nil || equalLines(lines, p.lines) {
		return false
	}
	p.lines = lines
	p.box.Objects = nil
	for _, line := range lines {
		text := canvas.NewText(line, p.color)
		text.TextSize = p.size
		text.TextStyle = p.style
		p.box.Add(text)
	}
	p.box.Refresh()
	return true
}

func (p *Paragraph) wrap(width float32) []string {
	return wrapLines(p.full, p.size, p.style, width)
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// wrapLines breaks text into the lines it takes at width: words are added to a
// line until the next one no longer fits. An empty line in the source stays an
// empty line.
//
// A lone separator is glued to the word after it, so a line never ends on the
// dot a "·"-separated list puts between its items.
func wrapLines(text string, size float32, style fyne.TextStyle, width float32) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := glueSeparators(strings.Fields(paragraph))
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			candidate := line + " " + word
			if fyne.MeasureText(candidate, size, style).Width <= width {
				line = candidate
				continue
			}
			lines = append(lines, line)
			line = word
		}
		lines = append(lines, line)
	}
	return lines
}

// glueSeparators joins a separator to the word that follows it - counted in
// runes, since a middle dot is two bytes - so that a line never ends on the dot
// a "·"-separated list puts between its items.
func glueSeparators(words []string) []string {
	glued := make([]string, 0, len(words))
	for i := 0; i < len(words); i++ {
		r, size := utf8.DecodeRuneInString(words[i])
		separator := size > 0 && size == len(words[i]) && !unicode.IsLetter(r) && !unicode.IsDigit(r)
		if separator && i+1 < len(words) {
			glued = append(glued, words[i]+" "+words[i+1])
			i++
			continue
		}
		glued = append(glued, words[i])
	}
	return glued
}

// lineHeight is what one line of text costs vertically.
func lineHeight(size float32, style fyne.TextStyle) float32 {
	return fyne.MeasureText("Ay", size, style).Height
}
