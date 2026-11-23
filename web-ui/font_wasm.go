//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"syscall/js"
	"time"

	"github.com/jeandeaual/go-locale"

	"amneziawg-web-ui/web-ui/internal/ui/style"
)

// fontPaths are the two faces the theme needs for Chinese, Japanese and Korean,
// as the server serves them next to the bundle. The build copies them out of
// web-ui/internal/ui/style/font/; they are not in the wasm binary.
var fontPaths = []string{
	"/font/NotoSansSC-Regular.otf",
	"/font/NotoSansSC-Bold.otf",
}

// installCJKFaces fetches the CJK faces when this browser's languages need
// them, and does nothing when they do not.
//
// The decision is made from navigator.languages, which is the same list Fyne
// uses to pick the panel's language, so the two cannot disagree about whether
// the page is Chinese. The fetch runs before the window is built, which is the
// price of never drawing a Chinese string as boxes: an English panel downloads
// 26 MB of wasm and stops there, a Chinese one downloads 12 MB of wasm and then
// the 15 MB of faces.
//
// A failure is returned rather than fatal: the panel works without them, simply
// with Fyne's Latin-only faces, which is still right for its own language.
func installCJKFaces() error {
	languages, err := locale.GetLocales()
	if err != nil {
		return fmt.Errorf("reading the browser's languages: %w", err)
	}
	if !style.NeedsCJK(languages) {
		return nil
	}

	regular, bold, err := fetchFaces()
	if err != nil {
		return err
	}
	style.SetFaces(regular, bold)
	return nil
}

// fetchFaces downloads both weights at once.
//
// The URLs are absolute because net/http wants them so: under js/wasm the
// request becomes a fetch() of that URL, and a relative path has no base to
// resolve against. The browser supplies the session cookie for the same origin
// and decompresses the gzip the image stores the files in, so what arrives here
// is the font itself.
func fetchFaces() (regular, bold []byte, err error) {
	origin := js.Global().Get("location").Get("origin").String()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	faces := make([][]byte, len(fontPaths))
	errs := make([]error, len(fontPaths))

	var wg sync.WaitGroup
	for i, path := range fontPaths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			faces[i], errs[i] = getBytes(ctx, origin+path)
		}()
	}
	wg.Wait()

	if err := firstError(errs); err != nil {
		return nil, nil, err
	}
	return faces[0], faces[1], nil
}

// firstError returns the first failure among the two downloads.
func firstError(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// getBytes is one GET, checked for a body worth installing in the theme. An
// empty file would parse into a face with no glyphs, which looks exactly like
// the failure this whole path exists to avoid.
func getBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck // a failed close says nothing about the font

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s: empty body", url)
	}
	return data, nil
}
