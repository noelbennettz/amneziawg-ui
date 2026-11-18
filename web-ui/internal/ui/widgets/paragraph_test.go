package widgets

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestWrapLinesKeepsEveryLineInsideTheWidth(t *testing.T) {
	test.NewApp()

	const width float32 = 120
	lines := wrapLines("Port 51898 · 10.66.0.0/24 · fd42:91ed::/64 · MTU 1280",
		12, fyne.TextStyle{Monospace: true}, width)

	if len(lines) < 2 {
		t.Fatalf("expected the line to break, got %q", lines)
	}
	for _, line := range lines {
		if got := fyne.MeasureText(line, 12, fyne.TextStyle{Monospace: true}).Width; got > width {
			t.Errorf("line %q is %.0f wide, past the %.0f it was wrapped for", line, got, width)
		}
	}
}

// A list separated by "·" must not leave the dot dangling at the end of a
// wrapped line: it belongs to the item that follows it.
func TestWrapLinesNeverEndsOnASeparator(t *testing.T) {
	test.NewApp()

	lines := wrapLines("Port 51898 · 10.66.0.0/24 · fd42:91ed::/64 · MTU 1280",
		12, fyne.TextStyle{Monospace: true}, 100)

	for _, line := range lines {
		if strings.HasSuffix(line, "·") {
			t.Errorf("line %q ends on a separator", line)
		}
	}
}

// The paragraph reports the height of the lines it actually draws at its
// minimum width, which is what keeps a long hint from making its card tall.
func TestParagraphMinSizeFollowsItsWrappedText(t *testing.T) {
	test.NewApp()

	short := NewParagraph("one line", 12, nil, fyne.TextStyle{}, 100)
	long := NewParagraph(strings.Repeat("a fairly long sentence that wraps ", 4),
		12, nil, fyne.TextStyle{}, 100)

	if short.MinSize().Width != 100 {
		t.Errorf("min width = %.0f, want the 100 it was given", short.MinSize().Width)
	}
	if long.MinSize().Height <= short.MinSize().Height {
		t.Errorf("long text is %.0f tall and the short one %.0f: the lines are not counted",
			long.MinSize().Height, short.MinSize().Height)
	}
}
