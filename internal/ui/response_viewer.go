package ui

import (
	"image"
	"image/color"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"tracto/internal/ui/syntax"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"golang.org/x/image/math/fixed"
)

// byteToRuneIdx walks `text` and returns the rune column reached at
// byteIdx (0-based count of complete runes preceding that byte). Used
// to map stored byte offsets (selection/highlight ranges) into the
// rune columns that gio's monospace layout actually paints — for
// 2-byte UTF-8 chars (Cyrillic etc.) byte_offset != visual_column,
// and using bytes for column math stretches selection rects past the
// rendered text.
func byteToRuneIdx(text []byte, byteIdx int) int {
	if byteIdx > len(text) {
		byteIdx = len(text)
	}
	n := 0
	i := 0
	for i < byteIdx {
		_, sz := utf8.DecodeRune(text[i:])
		if sz < 1 {
			sz = 1
		}
		i += sz
		n++
	}
	return n
}

// runeIdxToByte returns the byte offset of the start of the
// runeIdx-th rune in text. Inverse of byteToRuneIdx; used by
// hit-testing to convert a rune column (derived from pixel x) back
// into the byte offset stored in selStart/selEnd.
func runeIdxToByte(text []byte, runeIdx int) int {
	n := 0
	i := 0
	for n < runeIdx && i < len(text) {
		_, sz := utf8.DecodeRune(text[i:])
		if sz < 1 {
			sz = 1
		}
		i += sz
		n++
	}
	return i
}

// ResponseViewer is a read-only, viewport-virtualised text widget.
//
// widget.Editor pre-shapes the entire text into a persistent per-glyph
// index (text.Glyph + combinedPos, ~112 B per glyph). For a 3 MB response
// that's ~370 MB of live heap for the index alone, held for the whole
// session. Viewer sidesteps that entirely: it keeps the source bytes and
// a small []int index of line starts (one int per '\n'), and shapes only
// the lines currently inside the viewport. Per-frame memory cost is
// O(visible lines) rather than O(total glyphs).
//
// The API mirrors the subset of widget.Editor that tab.go actually uses
// for the response pane: SetText, Insert, Text, Len, plus scroll +
// highlight accessors. Caret/selection are degenerate (single byte-range
// highlight driven by search) since viewing is read-only.
type ResponseViewer struct {
	text       []byte
	lineStarts []int // byte offset of each chunk start (first entry always 0)

	// chunkHeights[i] is the pixel height that chunk i actually occupies
	// at the current viewport width and wrap mode. Initialised to the
	// nominal line height; replaced with the real value the first time a
	// chunk is rendered (Label.Layout returns its size). This lets us
	// virtualise correctly even though we don't pre-shape the whole
	// document — heights accumulate as the user scrolls past chunks.
	chunkHeights      []int
	chunkHeightsWrap  bool
	chunkHeightsWidth int

	scrollY int // pixel offset
	scrollX int // pixel offset (non-wrap mode)

	// maxLineWidth is the widest measured chunk in pixels. Used to clamp
	// horizontal scroll in non-wrap mode. Grows lazily as chunks render
	// — initially 0, so the first frame allows no horizontal scroll;
	// after the user moves around it converges to the true maximum.
	maxLineWidth int

	highlightStart int // byte offset of current highlight range
	highlightEnd   int

	// Mouse selection: byte offsets. selStart is the anchor (where the
	// drag began); selEnd follows the mouse. They may be in either
	// order; SelectedText normalises before slicing.
	selStart   int
	selEnd     int
	dragActive bool

	Scroller  gesture.Scroll // vertical
	ScrollerH gesture.Scroll // horizontal (only active in non-wrap mode)
	Drag      gesture.Drag   // mouse selection (motion)
	Click     gesture.Click  // mouse selection (press detection w/ NumClicks)

	// Computed each frame, surfaced via GetScrollBounds for the scroll bar.
	lastLineHeight int
	lastTotalH     int
	lastViewportH  int

	// Tokens for the whole document, computed once per (text, lang) pair
	// and looked up by chunk via the per-line index. The cache lives on
	// the viewer because lang only switches when a new response arrives
	// — across renders within a response the same token slice is reused.
	tokens     []syntax.Token
	tokensLang syntax.Lang
	tokensTxt  int // len(text) at the time tokens were last (re)built
}

func NewResponseViewer() *ResponseViewer {
	return &ResponseViewer{
		lineStarts: []int{0},
	}
}

// spansForChunk slices the document-wide token stream into the byte
// range [chunkStart, chunkEnd) and translates each token to a
// coloredSpan (color resolved from the active palette + bracket
// cycling). Token offsets are rebased to be chunk-local since
// paintColoredText walks the chunk text starting at byte 0.
func (v *ResponseViewer) spansForChunk(chunkStart, chunkEnd int, sp syntaxPalette, bracketCycle bool) []coloredSpan {
	if len(v.tokens) == 0 || chunkStart >= chunkEnd {
		return nil
	}
	// Binary-search the first token that touches the chunk. Tokens are
	// emitted in order so this is stable.
	first := sort.Search(len(v.tokens), func(i int) bool {
		return v.tokens[i].End > chunkStart
	})
	if first >= len(v.tokens) || v.tokens[first].Start >= chunkEnd {
		return nil
	}
	out := make([]coloredSpan, 0, 16)
	for i := first; i < len(v.tokens); i++ {
		t := v.tokens[i]
		if t.Start >= chunkEnd {
			break
		}
		s, e := t.Start, t.End
		if s < chunkStart {
			s = chunkStart
		}
		if e > chunkEnd {
			e = chunkEnd
		}
		if s >= e {
			continue
		}
		out = append(out, coloredSpan{
			Start: s - chunkStart,
			End:   e - chunkStart,
			Color: sp.colorForToken(t.Kind, t.Depth, bracketCycle),
		})
	}
	return out
}

// SetText replaces the viewer's content and resets scroll and highlight.
func (v *ResponseViewer) SetText(s string) {
	if cap(v.text) < len(s) {
		v.text = make([]byte, 0, len(s))
	}
	v.text = append(v.text[:0], s...)
	v.rebuildLineStartsFrom(0)
	v.invalidateChunkHeights()
	v.scrollY = 0
	v.scrollX = 0
	v.maxLineWidth = 0
	v.highlightStart = 0
	v.highlightEnd = 0
	v.selStart = 0
	v.selEnd = 0
	v.dragActive = false
}

// SelectedText returns the bytes between the user's selection anchor
// and current selection end. Empty when no selection. The Copy button
// in tab.go uses this when present, falling back to the full text.
func (v *ResponseViewer) SelectedText() string {
	if v.selStart == v.selEnd {
		return ""
	}
	s, e := v.selStart, v.selEnd
	if s > e {
		s, e = e, s
	}
	if s < 0 {
		s = 0
	}
	if e > len(v.text) {
		e = len(v.text)
	}
	return string(v.text[s:e])
}

// Append adds text to the end. Cheap: just extends the byte slice and
// appends new line-start offsets.
func (v *ResponseViewer) Append(s string) {
	startIdx := len(v.text)
	v.text = append(v.text, s...)
	v.appendLineStartsFrom(startIdx)
	// New chunks get default heights; existing ones are still valid.
	v.padChunkHeights()
}

func (v *ResponseViewer) invalidateChunkHeights() {
	v.chunkHeights = v.chunkHeights[:0]
}

// padChunkHeights extends chunkHeights with zero entries (placeholder
// "use default") so it stays the same length as lineStarts after Append.
func (v *ResponseViewer) padChunkHeights() {
	for len(v.chunkHeights) < len(v.lineStarts) {
		v.chunkHeights = append(v.chunkHeights, 0)
	}
	if len(v.chunkHeights) > len(v.lineStarts) {
		v.chunkHeights = v.chunkHeights[:len(v.lineStarts)]
	}
}

// Insert is an alias for Append. The streaming code treats the response
// editor as append-only, so we don't support mid-text insertion.
func (v *ResponseViewer) Insert(s string) { v.Append(s) }

// Text returns a string copy of the buffer. Uses the same unsafe-looking
// but standard pattern as gio's editor.
func (v *ResponseViewer) Text() string { return string(v.text) }

// Len returns the byte length of the buffer.
func (v *ResponseViewer) Len() int { return len(v.text) }

// Selection returns the current highlight range. Kept for API parity.
func (v *ResponseViewer) Selection() (int, int) {
	return v.highlightStart, v.highlightEnd
}

// SetCaret sets the highlight range and scrolls to bring it into view.
func (v *ResponseViewer) SetCaret(start, end int) {
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start > len(v.text) {
		start = len(v.text)
	}
	if end > len(v.text) {
		end = len(v.text)
	}
	v.highlightStart = start
	v.highlightEnd = end
	v.scrollToByteOffset(start)
}

// SetScrollCaret is a no-op; present only for widget.Editor API parity.
func (v *ResponseViewer) SetScrollCaret(bool) {}

// GetScrollY returns the current vertical scroll position in pixels.
func (v *ResponseViewer) GetScrollY() int { return v.scrollY }

// SetScrollY sets the vertical scroll position in pixels.
func (v *ResponseViewer) SetScrollY(y int) {
	v.scrollY = y
	v.clampScroll()
}

// GetScrollBounds returns the scrollable extents. Y extent is a running
// estimate based on the last-rendered line height; it stabilises once
// a frame has run.
func (v *ResponseViewer) GetScrollBounds() image.Rectangle {
	if v.lastLineHeight == 0 {
		return image.Rectangle{}
	}
	totalH := len(v.lineStarts) * v.lastLineHeight
	return image.Rectangle{Max: image.Point{Y: totalH}}
}

func (v *ResponseViewer) clampScroll() {
	if v.scrollY < 0 {
		v.scrollY = 0
	}
	if v.lastTotalH > 0 && v.lastViewportH > 0 {
		maxY := v.lastTotalH - v.lastViewportH
		if maxY < 0 {
			maxY = 0
		}
		if v.scrollY > maxY {
			v.scrollY = maxY
		}
	}
	if v.scrollX < 0 {
		v.scrollX = 0
	}
}

func (v *ResponseViewer) scrollToByteOffset(off int) {
	if v.lastLineHeight == 0 {
		return
	}
	line := v.lineForByteOffset(off)
	target := line * v.lastLineHeight
	// Center the match in the viewport when we know the viewport height.
	if v.lastViewportH > 0 {
		target -= v.lastViewportH / 2
	}
	v.scrollY = target
	v.clampScroll()
}

// lineForByteOffset returns the source-line index that contains off. O(log n).
func (v *ResponseViewer) lineForByteOffset(off int) int {
	lo, hi := 0, len(v.lineStarts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if v.lineStarts[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// chunkMaxBytes caps the size of any single display chunk. Without it,
// a minified JSON body (one multi-MB "line") would be handed to Label
// as a single string, and Label would ask the shaper to lay out the
// whole thing — re-creating exactly the allocation pattern we're trying
// to avoid. Chunking at 2 KiB keeps the per-frame shape cheap and bounds
// the cache-entry size for repeated renders. Word-wrap may break at
// chunk boundaries for long unbroken lines, which is acceptable for
// response bodies.
const chunkMaxBytes = 2048

func (v *ResponseViewer) rebuildLineStartsFrom(startIdx int) {
	// Truncate existing entries past startIdx, then re-scan.
	for len(v.lineStarts) > 0 && v.lineStarts[len(v.lineStarts)-1] > startIdx {
		v.lineStarts = v.lineStarts[:len(v.lineStarts)-1]
	}
	if len(v.lineStarts) == 0 {
		v.lineStarts = append(v.lineStarts, 0)
	}
	v.scanChunks(v.lineStarts[len(v.lineStarts)-1])
}

func (v *ResponseViewer) appendLineStartsFrom(startIdx int) {
	// Start scanning from the previous chunk's start so we correctly
	// apply chunkMaxBytes across the Append boundary.
	if len(v.lineStarts) == 0 {
		v.lineStarts = append(v.lineStarts, 0)
	}
	last := v.lineStarts[len(v.lineStarts)-1]
	if last > startIdx {
		last = startIdx
	}
	// Trim so we don't double-append chunks already recorded.
	for len(v.lineStarts) > 1 && v.lineStarts[len(v.lineStarts)-1] >= startIdx {
		v.lineStarts = v.lineStarts[:len(v.lineStarts)-1]
	}
	v.scanChunks(v.lineStarts[len(v.lineStarts)-1])
}

// scanChunks walks the text from position `from` to the end, appending a
// new chunk start every \n or every chunkMaxBytes bytes of continuous
// non-newline content. Forced breaks are pulled back to the nearest
// UTF-8 codepoint boundary so the resulting chunk is always valid
// UTF-8 — otherwise rendering replaces the broken byte with U+FFFD
// and the surrounding characters disappear.
func (v *ResponseViewer) scanChunks(from int) {
	lastBreak := from
	for i := from; i < len(v.text); i++ {
		if v.text[i] == '\n' {
			if i+1 <= len(v.text) {
				v.lineStarts = append(v.lineStarts, i+1)
			}
			lastBreak = i + 1
		} else if i-lastBreak >= chunkMaxBytes {
			breakAt := i
			// Walk back over UTF-8 continuation bytes (10xxxxxx) so the
			// chunk boundary lands on the start of a codepoint.
			for breakAt > lastBreak && (v.text[breakAt]&0xC0) == 0x80 {
				breakAt--
			}
			// If we couldn't find a codepoint start within the chunk,
			// fall back to the byte position to make forward progress.
			if breakAt == lastBreak {
				breakAt = i
			}
			v.lineStarts = append(v.lineStarts, breakAt)
			lastBreak = breakAt
		}
	}
}

// ResponseViewerStyle renders a ResponseViewer. Create one per Layout
// call — it's a value type holding only refs and style knobs.
type ResponseViewerStyle struct {
	Viewer         *ResponseViewer
	Shaper         *text.Shaper
	Font           font.Font
	TextSize       unit.Sp
	Color          color.NRGBA
	HighlightColor color.NRGBA // search match background
	SelectionColor color.NRGBA // mouse selection background
	Wrap           bool
	// Padding is applied inside the viewer's clip — text and selection
	// rectangles shift by Padding while gestures and scroll bounds still
	// cover the full clipped region. Same value applies to wrap and
	// non-wrap modes so users see consistent breathing room.
	Padding unit.Dp

	// Lang drives syntax coloring. LangPlain (the zero value) skips
	// tokenization entirely and renders in a single Color, matching the
	// pre-syntax-highlighting behaviour exactly.
	Lang syntax.Lang

	// Syntax is the per-token-kind palette used when Lang != LangPlain.
	// Brackets[depth%3] paints TokBracket when BracketCycle is true;
	// otherwise brackets fall through to Punctuation.
	Syntax        syntaxPalette
	BracketCycle  bool
}

func (s ResponseViewerStyle) Layout(gtx layout.Context) layout.Dimensions {
	v := s.Viewer

	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{Size: size}
	}

	// Refresh tokens when language switches OR the document was replaced
	// (SetText/Append both shift len(v.text); we only re-tokenize on a
	// length change, accepting that mid-text mutations would need a
	// stronger signal — but ResponseViewer is append-only in practice).
	if s.Lang != syntax.LangPlain {
		if s.Lang != v.tokensLang || len(v.text) != v.tokensTxt {
			v.tokens = syntax.Tokenize(s.Lang, v.text)
			v.tokensLang = s.Lang
			v.tokensTxt = len(v.text)
		}
	} else if v.tokens != nil {
		v.tokens = nil
		v.tokensLang = syntax.LangPlain
		v.tokensTxt = 0
	}

	pad := 0
	if s.Padding > 0 {
		pad = gtx.Dp(s.Padding)
	}
	// Guard against viewports smaller than 2×padding — fall back to no
	// padding so we never produce zero-or-negative inner dimensions.
	if pad*2 >= size.X || pad*2 >= size.Y {
		pad = 0
	}
	innerW := size.X - 2*pad
	innerH := size.Y - 2*pad

	// Line height derived from font size. 1.4× is gio's default LineHeightScale.
	lineHeight := gtx.Sp(s.TextSize) * 7 / 5
	if lineHeight <= 0 {
		lineHeight = 14
	}
	v.lastLineHeight = lineHeight
	v.lastViewportH = innerH

	// Reset measured heights when wrap mode or wrap width changes —
	// each combination produces different wrapped layouts. Then make
	// sure the heights slice tracks current chunk count. Also reset
	// the horizontal scroll bookkeeping on wrap-mode flip — the old
	// maxLineWidth is meaningless for the new mode.
	if s.Wrap != v.chunkHeightsWrap || (s.Wrap && v.chunkHeightsWidth != innerW) {
		v.invalidateChunkHeights()
		v.chunkHeightsWrap = s.Wrap
		v.chunkHeightsWidth = innerW
		v.maxLineWidth = 0
		v.scrollX = 0
	}
	v.padChunkHeights()

	// Pre-record textColor and measure the monospace metrics now,
	// before we use them for height estimates and scroll math. Both
	// measurements share gtx.Ops with the rest of the frame; we
	// discard the recorded "M" macro from measureLineHeight so it
	// never paints.
	textColorMacro := op.Record(gtx.Ops)
	paint.ColorOp{Color: s.Color}.Add(gtx.Ops)
	textColor := textColorMacro.Stop()

	charAdv := measureCharAdvance(s.Shaper, s.Font, s.TextSize, gtx)
	exactLineH := measureLineHeight(s.Shaper, s.Font, s.TextSize, textColor, gtx)
	if exactLineH <= 0 {
		exactLineH = lineHeight
	}
	v.lastLineHeight = exactLineH

	// Estimate total height. For unmeasured chunks use a per-chunk
	// estimate based on byte length and chars_per_line — way more
	// accurate than nominal lineHeight, which would convince the
	// virtualisation that ~40 chunks fit in the viewport on the
	// first frame and trigger a multi-hundred-ms freeze + cache
	// inflation rendering them.
	totalH := 0
	for i, h := range v.chunkHeights {
		if h > 0 {
			totalH += h
		} else {
			totalH += v.estimateChunkHeight(i, exactLineH, charAdv, innerW, s.Wrap)
		}
	}
	if totalH < innerH {
		totalH = innerH
	}
	v.lastTotalH = totalH

	// Process scroll input *before* clamping so wheel events can push us
	// into the newly-valid range when content grew.
	maxY := totalH - innerH
	if maxY < 0 {
		maxY = 0
	}
	sdist := v.Scroller.Update(
		gtx.Metric, gtx.Source, gtx.Now, gesture.Vertical,
		pointer.ScrollRange{},
		pointer.ScrollRange{Min: -v.scrollY, Max: maxY - v.scrollY},
	)
	v.scrollY += sdist

	// Horizontal scroll — only meaningful in non-wrap mode where lines
	// can extend past the viewport. maxLineWidth grows lazily as
	// chunks render; the first frame may have no scroll range, but it
	// catches up after the user moves to expose more content.
	if !s.Wrap {
		maxX := v.maxLineWidth - innerW
		if maxX < 0 {
			maxX = 0
		}
		sxdist := v.ScrollerH.Update(
			gtx.Metric, gtx.Source, gtx.Now, gesture.Horizontal,
			pointer.ScrollRange{Min: -v.scrollX, Max: maxX - v.scrollX},
			pointer.ScrollRange{},
		)
		v.scrollX += sxdist
		if v.scrollX > maxX {
			v.scrollX = maxX
		}
	} else {
		v.scrollX = 0
	}
	v.clampScroll()

	// Walk chunks until we accumulate up to the scroll position; that's
	// where the visible window begins. Use the per-chunk estimator
	// for unmeasured chunks so the search lands on the right line.
	firstLine, accumY := v.firstChunkAtFn(v.scrollY, exactLineH, charAdv, innerW, s.Wrap)

	// Clip to our viewport so lines partially above/below don't bleed out.
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()

	// Register the scroll + selection gestures and the keyboard focus
	// tag for the viewer (used by Ctrl+A / Ctrl+C key.Filter targets).
	// pointer.CursorText sets the I-beam over the entire clipped rect;
	// var chips below override it locally with CursorPointer.
	pointer.CursorText.Add(gtx.Ops)
	v.Scroller.Add(gtx.Ops)
	if !s.Wrap {
		v.ScrollerH.Add(gtx.Ops)
	}
	v.Drag.Add(gtx.Ops)
	v.Click.Add(gtx.Ops)
	event.Op(gtx.Ops, v)
	// Pull (and discard) Move events on the viewer's tag — gio only
	// applies cursor hints for areas that have an active pointer
	// target, so without an event subscription the I-beam stays
	// inactive even with CursorText.Add.
	for {
		_, ok := gtx.Event(pointer.Filter{Target: v, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
		if !ok {
			break
		}
	}

	// Push the inner-padding offset *after* registering gestures and the
	// event tag so they keep covering the full clipped rect (clicks in
	// the padding zone still focus the viewer and scroll-wheel still
	// works there). Everything below renders in inner coords.
	if pad > 0 {
		padTr := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
		defer padTr.Pop()
	}

	lbl := widget.Label{}
	if !s.Wrap {
		lbl.MaxLines = 1
	} else {
		// Force grapheme-boundary wrapping (hard wrap at any char) so
		// the visual layout matches our chars_per_line math used for
		// selection and search highlighting. With the default policy
		// (WrapHeuristic → soft wrap at spaces) gio produces shorter
		// lines than our math predicts, leaving selection rects
		// misaligned by half a line on some sub-lines and turning the
		// trailing whitespace into apparent extra characters.
		lbl.WrapPolicy = text.WrapGraphemes
	}

	hasSel := v.selStart != v.selEnd
	hasHL := v.highlightEnd > v.highlightStart

	// 1. Press detection via gesture.Click — exposes NumClicks and
	//    Modifiers so we can do native-editor double-click word /
	//    triple-click line / shift-click extend selection. Also where
	//    we request keyboard focus so subsequent Ctrl+A / Ctrl+C key
	//    events route to the viewer.
	for {
		ev, ok := v.Click.Update(gtx.Source)
		if !ok {
			break
		}
		if ev.Kind != gesture.KindPress || ev.Source != pointer.Mouse {
			continue
		}
		off := v.coordToByteOffset(ev.Position.X-pad, ev.Position.Y-pad, charAdv, exactLineH, innerW, s.Wrap)
		gtx.Execute(key.FocusCmd{Tag: v})
		switch {
		case ev.NumClicks >= 3:
			v.selStart, v.selEnd = v.sourceLineBoundsAt(off)
			v.dragActive = false
		case ev.NumClicks == 2:
			v.selStart, v.selEnd = v.wordBoundsAt(off)
			v.dragActive = false
		case ev.Modifiers&key.ModShift != 0:
			// Extend the existing selection's far end to the click
			// point. selStart stays as the anchor; selEnd follows.
			v.selEnd = off
			v.dragActive = true
		default:
			v.selStart = off
			v.selEnd = off
			v.dragActive = true
		}
		hasSel = v.selStart != v.selEnd
	}

	// 2. Drag motion to extend the selection in flight. Press is
	//    handled by Click above, so we only process Drag/Release/Cancel
	//    here — gesture.Drag still consumes the underlying Press
	//    event internally to track its own state, just don't act on
	//    it twice.
	for {
		ev, ok := v.Drag.Update(gtx.Metric, gtx.Source, gesture.Both)
		if !ok {
			break
		}
		switch ev.Kind {
		case pointer.Drag:
			if v.dragActive {
				off := v.coordToByteOffset(int(ev.Position.X)-pad, int(ev.Position.Y)-pad, charAdv, exactLineH, innerW, s.Wrap)
				v.selEnd = off
				hasSel = v.selStart != v.selEnd
			}
		case pointer.Release, pointer.Cancel:
			v.dragActive = false
			hasSel = v.selStart != v.selEnd
		}
	}

	// 3. Keyboard shortcuts when the viewer has focus. Modifier-shortcuts
	//    (Ctrl+A select-all, Ctrl+C copy, Ctrl+Home/End document jump,
	//    Ctrl+Left/Right word move) and bare navigation (arrows, Home,
	//    End, PageUp/PageDown) — Shift extends the existing selection;
	//    bare keypress collapses it onto the new caret position.
	for {
		// FocusFilter must be in the filter list — without it gio
		// silently ignores key.FocusCmd, leaving the viewer un-focused
		// and starving the named key.Filters below of events
		// (Ctrl+A / Ctrl+C / arrows etc. silently drop).
		ev, ok := gtx.Event(
			key.FocusFilter{Target: v},
			key.Filter{Focus: v, Name: "A", Required: key.ModShortcut},
			key.Filter{Focus: v, Name: "C", Required: key.ModShortcut},
			key.Filter{Focus: v, Name: key.NameLeftArrow, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NameRightArrow, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NameUpArrow, Optional: key.ModShift},
			key.Filter{Focus: v, Name: key.NameDownArrow, Optional: key.ModShift},
			key.Filter{Focus: v, Name: key.NameHome, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NameEnd, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NamePageUp, Optional: key.ModShift},
			key.Filter{Focus: v, Name: key.NamePageDown, Optional: key.ModShift},
		)
		if !ok {
			break
		}
		// Drain FocusEvent silently — we don't need any state change
		// (the viewer is read-only, no IME hookup) but we must consume
		// the event so the iteration progresses.
		if _, ok := ev.(key.FocusEvent); ok {
			continue
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		extend := ke.Modifiers.Contain(key.ModShift)
		wordwise := ke.Modifiers.Contain(key.ModShortcut)
		switch ke.Name {
		case "A":
			v.SelectAll()
		case "C":
			if sel := v.SelectedText(); sel != "" {
				gtx.Execute(clipboard.WriteCmd{
					Type: "application/text",
					Data: io.NopCloser(strings.NewReader(sel)),
				})
			}
		case key.NameLeftArrow:
			pos := v.selEnd
			if wordwise {
				pos = v.wordLeft(pos)
			} else {
				pos = v.charLeft(pos)
			}
			v.moveCaret(pos, extend)
			v.ensureCaretVisible()
		case key.NameRightArrow:
			pos := v.selEnd
			if wordwise {
				pos = v.wordRight(pos)
			} else {
				pos = v.charRight(pos)
			}
			v.moveCaret(pos, extend)
			v.ensureCaretVisible()
		case key.NameUpArrow:
			if s.Wrap {
				cpl := charsPerLineFor(innerW, charAdv)
				col := v.visualColumnAt(v.selEnd, cpl)
				v.moveCaret(v.wrapLineMove(v.selEnd, col, cpl, -1), extend)
			} else {
				col := v.columnAt(v.selEnd)
				v.moveCaret(v.lineUp(v.selEnd, col), extend)
			}
			v.ensureCaretVisible()
		case key.NameDownArrow:
			if s.Wrap {
				cpl := charsPerLineFor(innerW, charAdv)
				col := v.visualColumnAt(v.selEnd, cpl)
				v.moveCaret(v.wrapLineMove(v.selEnd, col, cpl, +1), extend)
			} else {
				col := v.columnAt(v.selEnd)
				v.moveCaret(v.lineDown(v.selEnd, col), extend)
			}
			v.ensureCaretVisible()
		case key.NameHome:
			if wordwise {
				v.moveCaret(0, extend)
			} else {
				lineStart, _ := v.sourceLineBoundsAt(v.selEnd)
				v.moveCaret(lineStart, extend)
			}
			v.ensureCaretVisible()
		case key.NameEnd:
			if wordwise {
				v.moveCaret(len(v.text), extend)
			} else {
				_, lineEnd := v.sourceLineBoundsAt(v.selEnd)
				v.moveCaret(lineEnd, extend)
			}
			v.ensureCaretVisible()
		case key.NamePageUp:
			lines := 1
			if v.lastLineHeight > 0 && v.lastViewportH > 0 {
				lines = v.lastViewportH / v.lastLineHeight
				if lines < 1 {
					lines = 1
				}
			}
			pos := v.selEnd
			if s.Wrap {
				cpl := charsPerLineFor(innerW, charAdv)
				col := v.visualColumnAt(pos, cpl)
				for i := 0; i < lines; i++ {
					newPos := v.wrapLineMove(pos, col, cpl, -1)
					if newPos == pos {
						break
					}
					pos = newPos
				}
			} else {
				col := v.columnAt(pos)
				for i := 0; i < lines; i++ {
					newPos := v.lineUp(pos, col)
					if newPos == pos {
						break
					}
					pos = newPos
				}
			}
			v.moveCaret(pos, extend)
			v.ensureCaretVisible()
		case key.NamePageDown:
			lines := 1
			if v.lastLineHeight > 0 && v.lastViewportH > 0 {
				lines = v.lastViewportH / v.lastLineHeight
				if lines < 1 {
					lines = 1
				}
			}
			pos := v.selEnd
			if s.Wrap {
				cpl := charsPerLineFor(innerW, charAdv)
				col := v.visualColumnAt(pos, cpl)
				for i := 0; i < lines; i++ {
					newPos := v.wrapLineMove(pos, col, cpl, +1)
					if newPos == pos {
						break
					}
					pos = newPos
				}
			} else {
				col := v.columnAt(pos)
				for i := 0; i < lines; i++ {
					newPos := v.lineDown(pos, col)
					if newPos == pos {
						break
					}
					pos = newPos
				}
			}
			v.moveCaret(pos, extend)
			v.ensureCaretVisible()
		}
		hasSel = v.selStart != v.selEnd
	}

	yOff := accumY - v.scrollY
	for line := firstLine; line < len(v.lineStarts); line++ {
		if yOff >= innerH {
			break
		}
		start, end := v.lineBounds(line)

		chunkH := v.chunkHeights[line]
		if chunkH == 0 {
			chunkH = v.estimateChunkHeight(line, exactLineH, charAdv, innerW, s.Wrap)
		}

		// Highlight rectangle behind chunk when search match overlaps.
		if hasHL && v.highlightEnd > start && v.highlightStart < end {
			v.paintHighlight(gtx, start, end, chunkH, yOff, charAdv, s.Wrap, innerW, s.HighlightColor, v.highlightStart, v.highlightEnd)
		}
		// Selection rectangle on top of (or instead of) the highlight.
		if hasSel {
			selS, selE := v.selStart, v.selEnd
			if selS > selE {
				selS, selE = selE, selS
			}
			if selE > start && selS < end {
				v.paintHighlight(gtx, start, end, chunkH, yOff, charAdv, s.Wrap, innerW, s.SelectionColor, selS, selE)
			}
		}

		tr := op.Offset(image.Pt(-v.scrollX, yOff)).Push(gtx.Ops)
		labelGtx := gtx
		labelGtx.Constraints.Min = image.Point{}
		if s.Wrap {
			labelGtx.Constraints.Max.X = innerW
		} else {
			// 1<<24 matches gio's SingleLine convention. math.MaxInt32/4
			// would overflow when the shaper converts MaxWidth to
			// fixed.Int26_6 (value << 6) — that's ~3.4e10, well past
			// int32, and the resulting garbage glyph metrics produce
			// nothing on screen.
			labelGtx.Constraints.Max.X = 1 << 24
		}
		labelGtx.Constraints.Max.Y = 1 << 24
		lineText := string(v.text[start:end])
		var dims layout.Dimensions
		if s.Lang != syntax.LangPlain && len(v.tokens) > 0 {
			spans := v.spansForChunk(start, end, s.Syntax, s.BracketCycle)
			dims = paintColoredText(labelGtx, s.Shaper, s.Font, s.TextSize, lineText, spans, s.Color, s.Wrap, innerW)
		} else {
			dims = lbl.Layout(labelGtx, s.Shaper, s.Font, s.TextSize, lineText, textColor)
		}
		tr.Pop()

		// Track widest measured chunk for horizontal scroll bounds.
		if !s.Wrap && dims.Size.X > v.maxLineWidth {
			v.maxLineWidth = dims.Size.X
		}

		// Record the actual measured height; future scroll math uses it.
		actualH := dims.Size.Y
		if actualH <= 0 {
			actualH = lineHeight
		}
		v.chunkHeights[line] = actualH
		yOff += actualH
	}

	return layout.Dimensions{Size: size}
}

// coordToByteOffset converts a viewer-local pointer position into a
// byte offset in v.text. Uses fixed-point char advance (matches gio's
// own layout) so the hit-test grid lines up with the rendered glyphs
// — int-rounded charW would mis-count by ~5–10 chars per viewport
// for non-integer advances and leave the trailing chars of every
// wrapped sub-line unselectable. Y math uses the same exact line
// height as the render loop, so the chunk found by the click is the
// one that actually appears at that pixel.
func (v *ResponseViewer) coordToByteOffset(
	posX, posY int,
	advance fixed.Int26_6,
	exactLineH, viewportW int,
	wrap bool,
) int {
	if advance <= 0 || exactLineH <= 0 || len(v.lineStarts) == 0 {
		return 0
	}
	yDoc := posY + v.scrollY
	if yDoc < 0 {
		yDoc = 0
	}

	// Find chunk by accumulating heights — measured where available,
	// estimated otherwise. This must use the same per-chunk values as
	// the render loop, or the byte-offset returned by clicking on
	// pixel y is for a chunk drawn somewhere else on screen.
	accum := 0
	chunkIdx := len(v.chunkHeights) - 1
	for i, h := range v.chunkHeights {
		if h <= 0 {
			h = v.estimateChunkHeight(i, exactLineH, advance, viewportW, wrap)
		}
		if accum+h > yDoc {
			chunkIdx = i
			break
		}
		accum += h
	}
	if chunkIdx < 0 || chunkIdx >= len(v.lineStarts) {
		return len(v.text)
	}
	chunkStart, chunkEnd := v.lineBounds(chunkIdx)
	chunkText := v.text[chunkStart:chunkEnd]
	chunkRunes := utf8.RuneCount(chunkText)

	if !wrap {
		// Column = (pixel + scroll) / advance, exact. The result is a
		// rune column (one per monospace glyph), then mapped back to
		// the byte offset of that rune so 2-byte UTF-8 chars don't
		// place the caret in the middle of a codepoint.
		col := int(fixed.I(posX+v.scrollX) / advance)
		if col < 0 {
			col = 0
		}
		if col > chunkRunes {
			col = chunkRunes
		}
		return chunkStart + runeIdxToByte(chunkText, col)
	}

	// Wrap: y within chunk → sub-line; x → rune column on that sub-line.
	yWithin := yDoc - accum
	if yWithin < 0 {
		yWithin = 0
	}
	wrapLine := yWithin / exactLineH
	charsPerLine := charsPerLineFor(viewportW, advance)
	col := int(fixed.I(posX) / advance)
	if col < 0 {
		col = 0
	}
	if col > charsPerLine {
		col = charsPerLine
	}
	runeIdx := wrapLine*charsPerLine + col
	if runeIdx > chunkRunes {
		runeIdx = chunkRunes
	}
	return chunkStart + runeIdxToByte(chunkText, runeIdx)
}

// estimateChunkHeight returns a best-guess pixel height for a chunk
// that hasn't been measured yet. In wrap mode this multiplies the
// per-line height by the number of sub-lines the chunk would
// produce at the current viewport width (chars_per_line computed
// from the exact fixed-point advance, matching gio's actual wrap
// math). Without a per-chunk estimate the loop in Layout assumes
// lineHeight (~18 px) for every unmeasured chunk, so on the first
// frame after a SetText/wrap-toggle/resize it renders ~40 chunks
// instead of the 2–3 that actually fit.
func (v *ResponseViewer) estimateChunkHeight(line, lineHeight int, advance fixed.Int26_6, viewportW int, wrap bool) int {
	if !wrap || advance <= 0 || viewportW <= 0 {
		return lineHeight
	}
	if line < 0 || line >= len(v.lineStarts) {
		return lineHeight
	}
	start := v.lineStarts[line]
	var end int
	if line+1 < len(v.lineStarts) {
		end = v.lineStarts[line+1]
	} else {
		end = len(v.text)
	}
	if end <= start {
		return lineHeight
	}
	// Sub-line count must be measured in runes (= visual columns for
	// monospace), not bytes. For non-ASCII chunks the byte length over-
	// counts columns by ~2× for Cyrillic and ~3× for CJK, predicting
	// 2–3× too many sub-lines and inflating the estimated chunk height
	// before the first measured render.
	chunkRunes := utf8.RuneCount(v.text[start:end])
	if chunkRunes <= 0 {
		return lineHeight
	}
	charsPerLine := charsPerLineFor(viewportW, advance)
	subLines := (chunkRunes + charsPerLine - 1) / charsPerLine
	if subLines < 1 {
		subLines = 1
	}
	return subLines * lineHeight
}

// charsPerLineFor returns the number of monospace chars that fit in
// the given viewport width using the shaper's exact fixed-point char
// advance. Matching gio's internal calculation is what keeps our
// hit-test grid aligned with the visual layout — round-then-divide
// with int pixels mis-counts by ~5–10 chars per viewport on
// non-integer advances (Ubuntu Mono at 13sp, etc.), so the last
// few chars of every wrapped sub-line ended up unselectable.
func charsPerLineFor(viewportW int, advance fixed.Int26_6) int {
	if advance <= 0 {
		return 1
	}
	n := int(fixed.I(viewportW) / advance)
	if n < 1 {
		n = 1
	}
	return n
}

// firstChunkAtFn returns the chunk index whose vertical range contains
// pixel offset y from the document top, plus the accumulated y where
// that chunk begins. Unmeasured chunks contribute estimateChunkHeight
// each (a per-chunk byte-length × chars_per_line projection).
func (v *ResponseViewer) firstChunkAtFn(y, lineH int, advance fixed.Int26_6, viewportW int, wrap bool) (int, int) {
	if y <= 0 {
		return 0, 0
	}
	accum := 0
	for i, h := range v.chunkHeights {
		if h <= 0 {
			h = v.estimateChunkHeight(i, lineH, advance, viewportW, wrap)
		}
		if accum+h > y {
			return i, accum
		}
		accum += h
	}
	return len(v.chunkHeights), accum
}

// lineBounds returns [start, end) byte offsets of display chunk n.
// Trailing '\n' and '\r' are stripped from end so they don't count as
// addressable columns: hit-testing inside coordToByteOffset clamps to
// `end`, and paintHighlight derives column-pixels from the same
// `end`. Without stripping, CRLF-encoded responses (typical HTTP)
// produce a phantom column past the visible text — the selection
// rectangle extends ~1 char wider than the rendered glyphs and the
// copied bytes include the lone '\r'.
func (v *ResponseViewer) lineBounds(n int) (int, int) {
	start := v.lineStarts[n]
	var end int
	if n+1 < len(v.lineStarts) {
		end = v.lineStarts[n+1]
	} else {
		end = len(v.text)
	}
	if end > start && v.text[end-1] == '\n' {
		end--
	}
	if end > start && v.text[end-1] == '\r' {
		end--
	}
	return start, end
}

// wordBoundsAt returns the byte offsets of the "word" containing
// byteOff for double-click selection, matching VS Code conventions:
//
//   - On a word character (letters, digits, underscore, hyphen, …):
//     select the contiguous run of non-separator runes. So clicking
//     on `m` of `"my-key"` returns the bounds of `my-key` — without
//     the surrounding quotes — and clicking on the hyphen still
//     selects the whole identifier.
//   - On whitespace: select the contiguous whitespace run.
//   - On any other separator (punctuation like `"`, `:`, `,`, `.`,
//     brackets, etc.): select just that one rune. This keeps each
//     punctuation char as its own click target instead of grouping
//     adjacent punctuation into one selection like `": "`.
func (v *ResponseViewer) wordBoundsAt(byteOff int) (int, int) {
	if byteOff < 0 {
		byteOff = 0
	}
	if byteOff >= len(v.text) {
		// EOF: walk back into the trailing word so double-click at
		// the very end of the file still selects the last word.
		byteOff = len(v.text)
		if byteOff == 0 {
			return 0, 0
		}
		prev, _ := utf8.DecodeLastRune(v.text[:byteOff])
		if isSeparator(prev) {
			return byteOff, byteOff
		}
		start := byteOff
		for start > 0 {
			r, sz := utf8.DecodeLastRune(v.text[:start])
			if isSeparator(r) {
				break
			}
			start -= sz
		}
		return start, byteOff
	}
	r, sz := utf8.DecodeRune(v.text[byteOff:])

	if isSeparator(r) {
		if unicode.IsSpace(r) {
			start := byteOff
			for start > 0 {
				prev, psz := utf8.DecodeLastRune(v.text[:start])
				if !unicode.IsSpace(prev) {
					break
				}
				start -= psz
			}
			end := byteOff
			for end < len(v.text) {
				next, nsz := utf8.DecodeRune(v.text[end:])
				if !unicode.IsSpace(next) {
					break
				}
				end += nsz
			}
			return start, end
		}
		// Punctuation rune — select just this one rune.
		return byteOff, byteOff + sz
	}

	// Word rune — select the contiguous non-separator run.
	start := byteOff
	for start > 0 {
		prev, psz := utf8.DecodeLastRune(v.text[:start])
		if isSeparator(prev) {
			break
		}
		start -= psz
	}
	end := byteOff
	for end < len(v.text) {
		next, nsz := utf8.DecodeRune(v.text[end:])
		if isSeparator(next) {
			break
		}
		end += nsz
	}
	return start, end
}

// sourceLineBoundsAt returns the byte offsets of the source-text line
// (between '\n's) containing byteOff. Used by triple-click line
// selection. The result excludes the trailing '\n' so triple-clicking
// inside a line and dragging into the next doesn't visually leak past
// the end of the highlighted line.
func (v *ResponseViewer) sourceLineBoundsAt(byteOff int) (int, int) {
	if byteOff < 0 {
		byteOff = 0
	}
	if byteOff > len(v.text) {
		byteOff = len(v.text)
	}
	start := byteOff
	for start > 0 && v.text[start-1] != '\n' {
		start--
	}
	end := byteOff
	for end < len(v.text) && v.text[end] != '\n' {
		end++
	}
	if end > start && v.text[end-1] == '\r' {
		end--
	}
	return start, end
}

// SelectAll selects every byte in the viewer's text. Wired to Ctrl+A
// when the viewer has keyboard focus. Public so the parent (e.g. tab
// toolbar buttons) could trigger it programmatically too.
func (v *ResponseViewer) SelectAll() {
	v.selStart = 0
	v.selEnd = len(v.text)
	v.dragActive = false
}

// moveCaret applies a byte-offset move to the caret, with optional
// selection extension. extend=false collapses any selection onto the
// new caret position; extend=true keeps selStart as the anchor and
// follows with selEnd, matching native editor convention.
func (v *ResponseViewer) moveCaret(newPos int, extend bool) {
	if newPos < 0 {
		newPos = 0
	}
	if newPos > len(v.text) {
		newPos = len(v.text)
	}
	if extend {
		v.selEnd = newPos
	} else {
		v.selStart = newPos
		v.selEnd = newPos
	}
	v.dragActive = false
}

// charLeft / charRight walk one rune at a time so the caret never
// lands inside a multi-byte UTF-8 codepoint.
func (v *ResponseViewer) charLeft(off int) int {
	if off <= 0 {
		return 0
	}
	_, sz := utf8.DecodeLastRune(v.text[:off])
	return off - sz
}

func (v *ResponseViewer) charRight(off int) int {
	if off >= len(v.text) {
		return len(v.text)
	}
	_, sz := utf8.DecodeRune(v.text[off:])
	return off + sz
}

// wordLeft / wordRight: move past adjacent separators, then over a
// run of word chars. Same separator definition as double-click word
// selection (isSeparator).
func (v *ResponseViewer) wordLeft(off int) int {
	if off <= 0 {
		return 0
	}
	i := off
	// Skip trailing separators.
	for i > 0 {
		r, sz := utf8.DecodeLastRune(v.text[:i])
		if !isSeparator(r) {
			break
		}
		i -= sz
	}
	// Skip the word.
	for i > 0 {
		r, sz := utf8.DecodeLastRune(v.text[:i])
		if isSeparator(r) {
			break
		}
		i -= sz
	}
	return i
}

func (v *ResponseViewer) wordRight(off int) int {
	if off >= len(v.text) {
		return len(v.text)
	}
	i := off
	// Skip current word.
	for i < len(v.text) {
		r, sz := utf8.DecodeRune(v.text[i:])
		if isSeparator(r) {
			break
		}
		i += sz
	}
	// Skip following separators.
	for i < len(v.text) {
		r, sz := utf8.DecodeRune(v.text[i:])
		if !isSeparator(r) {
			break
		}
		i += sz
	}
	return i
}

// columnAt returns the rune-column of off within its source line.
// Used as the "preferred column" when moving up/down so the caret
// stays in the same visual column across lines of varying length.
func (v *ResponseViewer) columnAt(off int) int {
	lineStart, _ := v.sourceLineBoundsAt(off)
	if off <= lineStart {
		return 0
	}
	return utf8.RuneCount(v.text[lineStart:off])
}

// offsetAtColumn returns the byte offset in the source line starting
// at lineStart that lies at the given rune-column, clamped to the
// line's end (so over-shooting on a short line lands at end-of-line
// rather than wrapping into the next).
func (v *ResponseViewer) offsetAtColumn(lineStart, col int) int {
	_, lineEnd := v.sourceLineBoundsAt(lineStart)
	if col <= 0 {
		return lineStart
	}
	off := lineStart
	runes := 0
	for off < lineEnd && runes < col {
		_, sz := utf8.DecodeRune(v.text[off:lineEnd])
		off += sz
		runes++
	}
	return off
}

// lineUp / lineDown move by source lines (one '\n'-delimited line at
// a time). In wrap mode this still moves a whole source line per
// keypress rather than a wrapped sub-line — simpler than tracking
// visual lines and matches what most users actually want when paging
// through structured payloads (JSON keys etc.).
func (v *ResponseViewer) lineUp(off, col int) int {
	lineStart, _ := v.sourceLineBoundsAt(off)
	if lineStart == 0 {
		return 0
	}
	prevLineStart, _ := v.sourceLineBoundsAt(lineStart - 1)
	return v.offsetAtColumn(prevLineStart, col)
}

func (v *ResponseViewer) lineDown(off, col int) int {
	_, lineEnd := v.sourceLineBoundsAt(off)
	if lineEnd >= len(v.text) {
		return len(v.text)
	}
	// lineEnd is right before a '\n'; skip it to land on the next line.
	nextLineStart := lineEnd + 1
	if nextLineStart > len(v.text) {
		nextLineStart = len(v.text)
	}
	return v.offsetAtColumn(nextLineStart, col)
}

// visualColumnAt returns the rune-column of off within its visual
// (wrapped) sub-line. Used as the "preferred visual column" when
// moving up/down in wrap mode so the caret stays in the same x even
// across sub-lines of varying length. cpl must be >0.
func (v *ResponseViewer) visualColumnAt(off, cpl int) int {
	if cpl < 1 {
		return 0
	}
	line := v.lineForByteOffset(off)
	chunkStart, chunkEnd := v.lineBounds(line)
	chunkText := v.text[chunkStart:chunkEnd]
	inChunkByte := off - chunkStart
	if inChunkByte < 0 {
		inChunkByte = 0
	}
	inChunkRune := byteToRuneIdx(chunkText, inChunkByte)
	return inChunkRune % cpl
}

// wrapLineMove moves one *visual* line in dir (-1 up, +1 down),
// keeping the caret near prefVisualCol. Splits the document by chunk
// (lineStarts) and within a chunk by sub-line of width cpl. Falls
// back to source-line motion when wrap is off (cpl<=0).
func (v *ResponseViewer) wrapLineMove(off, prefVisualCol, cpl, dir int) int {
	if cpl < 1 {
		// Without a wrap width, fall back to source-line motion.
		if dir < 0 {
			return v.lineUp(off, prefVisualCol)
		}
		return v.lineDown(off, prefVisualCol)
	}
	clampInChunk := func(start, end, target int) int {
		text := v.text[start:end]
		runes := utf8.RuneCount(text)
		if target > runes {
			target = runes
		}
		if target < 0 {
			target = 0
		}
		return start + runeIdxToByte(text, target)
	}

	line := v.lineForByteOffset(off)
	chunkStart, chunkEnd := v.lineBounds(line)
	chunkText := v.text[chunkStart:chunkEnd]
	chunkRunes := utf8.RuneCount(chunkText)
	inChunkRune := byteToRuneIdx(chunkText, off-chunkStart)
	subLine := 0
	if cpl > 0 {
		subLine = inChunkRune / cpl
	}

	if dir < 0 {
		if subLine > 0 {
			return clampInChunk(chunkStart, chunkEnd, (subLine-1)*cpl+prefVisualCol)
		}
		if line == 0 {
			return 0
		}
		prevStart, prevEnd := v.lineBounds(line - 1)
		prevRunes := utf8.RuneCount(v.text[prevStart:prevEnd])
		lastSub := 0
		if prevRunes > 0 {
			lastSub = (prevRunes - 1) / cpl
		}
		return clampInChunk(prevStart, prevEnd, lastSub*cpl+prefVisualCol)
	}
	lastSubInChunk := 0
	if chunkRunes > 0 {
		lastSubInChunk = (chunkRunes - 1) / cpl
	}
	if subLine < lastSubInChunk {
		return clampInChunk(chunkStart, chunkEnd, (subLine+1)*cpl+prefVisualCol)
	}
	if line+1 >= len(v.lineStarts) {
		return len(v.text)
	}
	nextStart, nextEnd := v.lineBounds(line + 1)
	return clampInChunk(nextStart, nextEnd, prefVisualCol)
}

// ensureCaretVisible adjusts scrollY so the caret's source line sits
// inside the viewport. Approximate — uses lastLineHeight for the line
// pitch since that's what the render loop uses for unmeasured chunks.
func (v *ResponseViewer) ensureCaretVisible() {
	if v.lastLineHeight == 0 {
		return
	}
	line := v.lineForByteOffset(v.selEnd)
	caretY := line * v.lastLineHeight
	if caretY < v.scrollY {
		v.scrollY = caretY
	} else if v.lastViewportH > 0 && caretY+v.lastLineHeight > v.scrollY+v.lastViewportH {
		v.scrollY = caretY + v.lastLineHeight - v.lastViewportH
	}
	v.clampScroll()
}

// paintHighlight draws a rectangle behind the byte range
// [rangeStart, rangeEnd) within the chunk rect. Uses the exact
// fixed-point char advance (matches gio's layout) so the rectangle
// lines up with the visual glyphs. Monospace assumption.
//
// In wrap mode the highlight may span multiple wrapped sub-lines; we
// emit one rect per affected sub-line. In non-wrap mode it's always a
// single rectangle.
//
// Used by both search highlight and mouse selection — the same
// machinery, just different colours and ranges.
func (v *ResponseViewer) paintHighlight(
	gtx layout.Context,
	chunkStart, chunkEnd int,
	chunkH, yOff int,
	advance fixed.Int26_6,
	wrap bool,
	viewportW int,
	col color.NRGBA,
	rangeStart, rangeEnd int,
) {
	if advance <= 0 {
		return
	}
	hStartByte := rangeStart - chunkStart
	if hStartByte < 0 {
		hStartByte = 0
	}
	maxEndByte := chunkEnd - chunkStart
	hEndByte := rangeEnd - chunkStart
	if hEndByte > maxEndByte {
		hEndByte = maxEndByte
	}
	if hEndByte <= hStartByte {
		return
	}
	// Selection ranges are stored as byte offsets but the rendered
	// glyphs lay out per rune (monospace = one advance per rune). For
	// multi-byte UTF-8 chars (Cyrillic, etc.) byte offset != visual
	// column, so do all column math in runes.
	chunkText := v.text[chunkStart:chunkEnd]
	hStart := byteToRuneIdx(chunkText, hStartByte)
	hEnd := byteToRuneIdx(chunkText, hEndByte)
	if hEnd <= hStart {
		return
	}
	colToPx := func(c int) int {
		return (advance * fixed.Int26_6(c)).Round()
	}
	// True when the selection extends past this chunk's last byte —
	// i.e. there are more chunks below that also belong to the
	// selection. Used to extend the chunk's last painted sub-line
	// down to chunkBottom (closing the vertical gap to the next
	// chunk) WITHOUT widening it past the actual text — the rect
	// stays bounded by `colToPx(endCol)` horizontally so empty space
	// between the last printable glyph and the line break is left
	// unpainted, matching non-wrap behavior. Strict `>` (not `>=`)
	// so when rangeEnd hits chunkEnd exactly the rect terminates at
	// the chunk's natural sub-line bottom.
	continuesPastChunk := rangeEnd > chunkEnd

	if !wrap {
		x1 := colToPx(hStart) - v.scrollX
		x2 := colToPx(hEnd) - v.scrollX
		r := image.Rect(x1, yOff, x2, yOff+chunkH)
		paint.FillShape(gtx.Ops, col, clip.Rect(r).Op())
		return
	}

	charsPerLine := charsPerLineFor(viewportW, advance)
	startWL := hStart / charsPerLine
	endWL := (hEnd - 1) / charsPerLine
	startCol := hStart % charsPerLine
	endCol := ((hEnd - 1) % charsPerLine) + 1

	// gio.Label with WrapPolicy=WrapGraphemes places each soft-wrapped
	// sub-line at exact lineHeight increments from the chunk's top.
	// Use that measured line height directly — deriving subLineH from
	// chunkH/numSubLines pulls in any descent/padding the chunk has
	// after its last sub-line and produces 0.5–1.5-line vertical drift.
	subLineH := v.lastLineHeight
	if subLineH < 1 {
		return
	}
	chunkBottom := yOff + chunkH
	// Full-width sub-line stops at the exact pixel where the
	// charsPerLine-th glyph ends — not at viewportW. With non-integer
	// fixed-point advance, those values differ by a few pixels and
	// filling to viewportW leaves a visible "phantom space" strip past
	// the last glyph on every intermediate sub-line.
	fullWidth := colToPx(charsPerLine)

	for wl := startWL; wl <= endWL; wl++ {
		y1 := yOff + wl*subLineH
		if y1 >= chunkBottom {
			break
		}
		y2 := y1 + subLineH
		// On the chunk's last painted sub-line, when the selection
		// continues into the chunk below, extend the rect's bottom
		// edge to chunkBottom so it touches the next chunk's first
		// sub-line. gio's rendered chunk height is `(N-1)*lineHeight
		// + ascent + descent` rather than `N*lineHeight`, so when
		// `ascent+descent > lineHeight` the natural y1+subLineH falls
		// short of the chunk's actual bottom — the strip between is
		// the inter-line leading area, and leaving it unpainted shows
		// up as a horizontal seam between the per-line selection
		// bands.
		if wl == endWL && continuesPastChunk {
			y2 = chunkBottom
		} else if y2 > chunkBottom {
			y2 = chunkBottom
		}
		x1 := 0
		x2 := fullWidth
		if wl == startWL {
			x1 = colToPx(startCol)
		}
		if wl == endWL {
			// Always clamp horizontally to the actual text on the
			// chunk's last sub-line — matches non-wrap behavior and
			// keeps empty horizontal space (between the last
			// printable glyph and the line break) out of the
			// selection.
			x2 = colToPx(endCol)
		}
		r := image.Rect(x1, y1, x2, y2)
		paint.FillShape(gtx.Ops, col, clip.Rect(r).Op())
	}
}

// measureCharAdvance returns the exact (fixed-point) pixel advance of
// "M" at the given font/size — i.e. the width gio uses internally
// when laying out monospace text. Returning the unrounded value lets
// callers compute chars-per-line and column→pixel math the same way
// gio does, instead of accumulating one-pixel rounding errors per
// column that drift the selection rect off the actual glyphs.
// Cached by the shaper's layoutCache (single short string).
func measureCharAdvance(shaper *text.Shaper, fnt font.Font, size unit.Sp, gtx layout.Context) fixed.Int26_6 {
	shaper.LayoutString(text.Parameters{
		Font:    fnt,
		PxPerEm: fixed.I(gtx.Sp(size)),
	}, "M")
	g, ok := shaper.NextGlyph()
	if !ok {
		return 0
	}
	return g.Advance
}

// measureLineHeight returns the inter-baseline pixel distance gio
// uses when stacking wrapped sub-lines — i.e. the value that lines up
// with the actual sub-line tops in a multi-line widget.Label render.
//
// This is NOT the same as `single_line_label.Size.Y`, which equals
// only ascent+descent of one line. The shaper places consecutive
// baselines `lineHeight` apart, where lineHeight = ascent + descent +
// lineGap (font metric). For monospace fonts with a non-zero lineGap
// the difference is several pixels; using ascent+descent for sub-line
// stacking drifts the selection rect up by lineGap × sub-line index,
// reaching half-to-full-line offsets after two or three sub-lines.
//
// We extract the true inter-line distance by laying out one and two
// lines and subtracting: dims2 = ascent + lineHeight + descent;
// dims1 = ascent + descent; dims2 - dims1 = lineHeight.
func measureLineHeight(
	shaper *text.Shaper,
	fnt font.Font,
	size unit.Sp,
	textColor op.CallOp,
	gtx layout.Context,
) int {
	measure := func(maxLines int, txt string) int {
		macro := op.Record(gtx.Ops)
		l := widget.Label{MaxLines: maxLines}
		lg := gtx
		lg.Constraints.Min = image.Point{}
		lg.Constraints.Max.X = 1 << 24
		lg.Constraints.Max.Y = 1 << 24
		dims := l.Layout(lg, shaper, fnt, size, txt, textColor)
		macro.Stop()
		return dims.Size.Y
	}
	h1 := measure(1, "M")
	h2 := measure(2, "M\nM")
	if h2 > h1 {
		return h2 - h1
	}
	return h1
}
