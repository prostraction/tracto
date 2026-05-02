package ui

import (
	"image"
	"image/color"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"tracto/internal/ui/syntax"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/io/transfer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"golang.org/x/image/math/fixed"
)


// RequestEditor is an editable, viewport-virtualised text widget — a
// drop-in for widget.Editor in scenarios where the body can grow
// well past widget.Editor's comfortable working size (large JSON
// payloads, file-backed bodies up to ~100 MB).
//
// widget.Editor pre-shapes the entire text into a persistent per-glyph
// index (~112 B per glyph), which for a 10 MB body costs gigabytes of
// heap and several seconds per layout. RequestEditor stores the raw
// bytes plus a []int line-start index and shapes only the lines that
// fall inside the viewport — per-frame cost is O(visible lines).
//
// Editing operations: Insert(pos, s), DeleteRange(start, end),
// Replace(start, end, s). text input + IME, undo/redo, caret blink
// and {{var}} highlighting are layered on top in subsequent commits.
type RequestEditor struct {
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

	// IME composing region. While the user is mid-composition (e.g. a
	// pinyin/hiragana sequence), the system reports the partial text via
	// SnippetEvent with a non-empty range; we mirror it here so the
	// render pass can underline / tint that span. Stored in byte
	// offsets even though gio's IME API works in rune indices —
	// conversion happens at the boundary.
	imeStart int
	imeEnd   int
	// imeSentSnippet remembers the last snippet pushed via SnippetCmd
	// so we don't re-push identical state every frame.
	imeSentSnippet key.Snippet

	// blinkStart marks the moment the caret last "did something" — we
	// hold the caret solid for a beat after every move/edit so the user
	// can track it, then resume blinking. Reset on FocusEvent and on
	// every buffer mutation.
	blinkStart time.Time

	// undo / redo diff stacks. Each entry stores only the bytes that
	// changed (deleted ↔ inserted), not a full snapshot — critical
	// for 100 MB bodies where 100 snapshots would otherwise cost
	// gigabytes. SetText clears both stacks (it's a fresh history).
	// Replay is straightforward: undo restores `deleted` at `pos` and
	// removes `inserted`; redo does the inverse.
	undoStack []editOp
	redoStack []editOp
	// suppressHistory pauses undo/redo recording. Used while
	// undoing/redoing so the inverse mutation doesn't itself land on
	// the stack and stop the chain.
	suppressHistory bool

	// dirty is set on every buffer mutation and cleared by Changed().
	// Mirrors widget.Editor's ChangeEvent contract so callers (tab.go)
	// can keep using the same dirty/system-headers polling pattern.
	dirty bool

	// oversizeMsg is set by Insert/SetText/LoadFromFile when an input
	// would push the buffer past RequestBodyMaxBytes. The UI shows it
	// as a banner with a "Load from file" affordance until the user
	// dismisses it or successfully attaches a file.
	oversizeMsg string

	// Tokens for the whole document. Recomputed on (lang, len(text))
	// change — the latter is a coarse signal for "the buffer was
	// mutated" that's cheap to check and avoids per-keystroke retoken
	// during typing. Past the soft cap below we skip tokenization
	// entirely and fall back to the single-color renderer, so a 50 MB
	// paste doesn't spend hundreds of ms tokenizing on every wrap-mode
	// reflow.
	tokens     []syntax.Token
	tokensLang syntax.Lang
	tokensTxt  int
}

// requestEditorTokenizeMaxBytes caps how large a buffer the editor will
// tokenize for syntax coloring. Beyond it, RequestEditorStyle.Layout
// renders single-color (still respects {{var}} chips). Picked at 1 MB —
// JSON request bodies that big are rare; people sending them care about
// throughput more than coloring.
const requestEditorTokenizeMaxBytes = 1 * 1024 * 1024

type editOp struct {
	pos       int
	deleted   []byte
	inserted  []byte
	selBefore int  // caret/selection anchor before the edit
	endBefore int  // caret end before the edit
	selAfter  int  // caret position after the edit
}

const requestEditorUndoLimit = 1000

// RequestBodyMaxBytes is the hard ceiling on the body the editor
// will hold in memory. Beyond this, callers should hand the body off
// to a file-backed transmit path (the request stays attached to a
// file path, the editor shows a placeholder). 100 MB matches the
// product target ("ожидаем до 1MB в 95% случаев"; 100 MB is the
// power-user upper bound).
const RequestBodyMaxBytes = 100 * 1024 * 1024

// requestEditorVarScanCutoff disables {{var}} highlighting once the
// buffer grows past this size. Var scanning is per-visible-chunk so
// it stays cheap even on huge bodies, but for >10 MB the scan starts
// to dominate frame time — and at that scale the body is almost
// certainly machine-generated, so per-var feedback is less useful
// than smooth scrolling.
const requestEditorVarScanCutoff = 10 * 1024 * 1024

func NewRequestEditor() *RequestEditor {
	return &RequestEditor{
		lineStarts: []int{0},
	}
}

// spansForChunk slices the document-wide token stream into the byte
// range [chunkStart, chunkEnd) and rebases offsets to be chunk-local
// (paintColoredText walks chunk text starting at byte 0). Mirrors
// ResponseViewer.spansForChunk; kept separate so the two viewer types
// don't have to share a base struct.
func (v *RequestEditor) spansForChunk(chunkStart, chunkEnd int, sp syntaxPalette, bracketCycle bool) []coloredSpan {
	if len(v.tokens) == 0 || chunkStart >= chunkEnd {
		return nil
	}
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

// SetText replaces the editor's content and resets scroll/highlight.
// Programmatic content swap — clears undo/redo because the new text
// is not a continuation of the previous editing session. Returns
// false if the input exceeds RequestBodyMaxBytes (the caller is
// expected to switch to a file-backed body in that case).
func (v *RequestEditor) SetText(s string) bool {
	if len(s) > RequestBodyMaxBytes {
		v.oversizeMsg = "Body exceeds 100 MB. Load from file instead."
		return false
	}
	v.oversizeMsg = ""
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
	v.undoStack = v.undoStack[:0]
	v.redoStack = v.redoStack[:0]
	return true
}

// IsOverSoftLimit reports whether the body has crossed the size at
// which UI should suggest switching to a file-backed payload.
func (v *RequestEditor) IsOverSoftLimit() bool {
	return len(v.text) >= RequestBodyMaxBytes
}

// OversizeMsg returns the latest reason an Insert/SetText/Load call
// was rejected for size, or "" when no rejection is pending. The UI
// shows it as a banner; DismissOversize clears it.
func (v *RequestEditor) OversizeMsg() string { return v.oversizeMsg }

// DismissOversize clears the over-limit banner. Called by the UI's
// "OK / dismiss" affordance.
func (v *RequestEditor) DismissOversize() { v.oversizeMsg = "" }

// SizeBytes returns the current buffer length.
func (v *RequestEditor) SizeBytes() int { return len(v.text) }

// LoadFromReader pulls bytes from r (up to RequestBodyMaxBytes+1 to
// detect overflow) and replaces the buffer with them. Used by the
// over-limit banner's "Load from file" affordance, which gets a
// ReadCloser from explorer.ChooseFile rather than a filesystem path.
func (v *RequestEditor) LoadFromReader(r io.Reader) error {
	limited := io.LimitReader(r, int64(RequestBodyMaxBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		v.oversizeMsg = "Load failed: " + err.Error()
		return err
	}
	if len(data) > RequestBodyMaxBytes {
		v.oversizeMsg = "File exceeds 100 MB; cannot load inline."
		return errBodyTooLarge
	}
	if !v.SetText(string(data)) {
		return errBodyTooLarge
	}
	return nil
}

// LoadFromFile reads `path` into the editor as a fresh document.
// Files past RequestBodyMaxBytes are rejected with an error so the
// caller can wire the file directly into the request transmit path
// instead of pulling its bytes through the editor.
func (v *RequestEditor) LoadFromFile(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		v.oversizeMsg = "Load failed: " + err.Error()
		return err
	}
	if fi.Size() > int64(RequestBodyMaxBytes) {
		v.oversizeMsg = "File exceeds 100 MB; cannot load inline."
		return errBodyTooLarge
	}
	data, err := os.ReadFile(path)
	if err != nil {
		v.oversizeMsg = "Load failed: " + err.Error()
		return err
	}
	if !v.SetText(string(data)) {
		return errBodyTooLarge
	}
	return nil
}

var errBodyTooLarge = errBody("request body exceeds 100 MB; load directly via the request's file source instead")

type errBody string

func (e errBody) Error() string { return string(e) }

// SelectedText returns the bytes between the user's selection anchor
// and current selection end. Empty when no selection. The Copy button
// in tab.go uses this when present, falling back to the full text.
func (v *RequestEditor) SelectedText() string {
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
func (v *RequestEditor) Append(s string) {
	startIdx := len(v.text)
	v.text = append(v.text, s...)
	v.appendLineStartsFrom(startIdx)
	// New chunks get default heights; existing ones are still valid.
	v.padChunkHeights()
}

func (v *RequestEditor) invalidateChunkHeights() {
	v.chunkHeights = v.chunkHeights[:0]
}

// padChunkHeights extends chunkHeights with zero entries (placeholder
// "use default") so it stays the same length as lineStarts after Append.
func (v *RequestEditor) padChunkHeights() {
	for len(v.chunkHeights) < len(v.lineStarts) {
		v.chunkHeights = append(v.chunkHeights, 0)
	}
	if len(v.chunkHeights) > len(v.lineStarts) {
		v.chunkHeights = v.chunkHeights[:len(v.lineStarts)]
	}
}

// Insert places s at byte offset pos and updates lineStarts /
// chunkHeights / maxLineWidth so subsequent renders see the new
// content. Out-of-range pos values are clamped to [0, len(text)].
//
// Implementation detail: lineStarts is rebuilt from the start of the
// chunk containing pos, not from byte 0. For 1 MB bodies this is a
// fraction of a millisecond per keystroke; for 100 MB it can be
// noticeable but still workable. A future optimisation could shift
// existing lineStarts entries past pos by len(s) and only rescan the
// modified chunk, but the current naive rebuild is correct and easy
// to reason about.
func (v *RequestEditor) Insert(pos int, s string) {
	if s == "" {
		return
	}
	// Reject mutations that would push the buffer past 100 MB. Callers
	// that legitimately want a larger body should use LoadFromFile and
	// the file-backed transmit path; inline editing past 100 MB
	// degrades layout/IME/undo into uselessness anyway.
	if len(v.text)+len(s) > RequestBodyMaxBytes {
		v.oversizeMsg = "Paste rejected: would exceed 100 MB. Load from file instead."
		return
	}
	if pos < 0 {
		pos = 0
	}
	if pos > len(v.text) {
		pos = len(v.text)
	}
	selBefore, endBefore := v.selStart, v.selEnd
	// Splice the bytes in.
	v.text = append(v.text[:pos], append([]byte(s), v.text[pos:]...)...)

	// Shift selection / highlight that sits past the insertion point.
	shift := len(s)
	v.shiftRanges(pos, shift)

	v.rebuildLineStartsFrom(pos)
	v.maxLineWidth = 0
	v.padChunkHeights()
	v.recordEdit(editOp{
		pos:       pos,
		deleted:   nil,
		inserted:  []byte(s),
		selBefore: selBefore,
		endBefore: endBefore,
		selAfter:  pos + len(s),
	})
}

// DeleteRange removes bytes in [start, end) and adjusts indices.
// Out-of-range / inverted args are clamped + sorted.
func (v *RequestEditor) DeleteRange(start, end int) {
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end > len(v.text) {
		end = len(v.text)
	}
	if start == end {
		return
	}
	selBefore, endBefore := v.selStart, v.selEnd
	deletedCopy := make([]byte, end-start)
	copy(deletedCopy, v.text[start:end])
	v.text = append(v.text[:start], v.text[end:]...)

	v.shiftRanges(end, -(end - start))

	v.rebuildLineStartsFrom(start)
	v.maxLineWidth = 0
	v.padChunkHeights()
	v.recordEdit(editOp{
		pos:       start,
		deleted:   deletedCopy,
		inserted:  nil,
		selBefore: selBefore,
		endBefore: endBefore,
		selAfter:  start,
	})
}

// Replace is DeleteRange(start, end) followed by Insert(start, s)
// recorded as a single undo step. We toggle suppressHistory around the
// inner calls so they don't push their own (now redundant) entries,
// then record the combined op manually.
func (v *RequestEditor) Replace(start, end int, s string) {
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end > len(v.text) {
		end = len(v.text)
	}
	selBefore, endBefore := v.selStart, v.selEnd
	var deletedCopy []byte
	if end > start {
		deletedCopy = make([]byte, end-start)
		copy(deletedCopy, v.text[start:end])
	}
	v.suppressHistory = true
	v.DeleteRange(start, end)
	v.Insert(start, s)
	v.suppressHistory = false
	if len(deletedCopy) == 0 && s == "" {
		return
	}
	v.recordEdit(editOp{
		pos:       start,
		deleted:   deletedCopy,
		inserted:  []byte(s),
		selBefore: selBefore,
		endBefore: endBefore,
		selAfter:  start + len(s),
	})
}

// recordEdit pushes the op onto the undo stack and clears redo (a
// fresh edit invalidates the redo timeline). No-op while replaying.
// Also flips the dirty flag so callers polling Changed() pick up the
// edit on the next frame.
func (v *RequestEditor) recordEdit(op editOp) {
	v.dirty = true
	if v.suppressHistory {
		return
	}
	// Try to merge with the previous step. Without grouping, every
	// keystroke becomes its own undo entry — typing "hello" then
	// Ctrl+Z removes one letter at a time, which feels broken.
	// Editors group adjacent same-kind edits up to a "type break"
	// (whitespace, newline, navigation, paste).
	if n := len(v.undoStack); n > 0 && canMergeEdit(v.undoStack[n-1], op) {
		mergeEditInto(&v.undoStack[n-1], op)
	} else {
		v.undoStack = append(v.undoStack, op)
	}
	if len(v.undoStack) > requestEditorUndoLimit {
		v.undoStack = v.undoStack[len(v.undoStack)-requestEditorUndoLimit:]
	}
	v.redoStack = v.redoStack[:0]
}

// canMergeEdit decides whether op continues the previous step in a
// way that should be undone in one shot.
//
// Rules (kept conservative — better to err on the side of "two undo
// steps" than to swallow user-distinguishable actions into one):
//
//   - Both must be pure insertions OR both must be pure deletions
//     (Replace ops, recorded with both deleted+inserted, never merge).
//   - Insertions merge only when contiguous (op.pos == prev.pos +
//     len(prev.inserted)) and neither inserted blob contains '\n' or
//     '\t' — newlines and tabs make natural "type breaks".
//   - Deletions merge in two flavours:
//       * Backspace chain: op.pos + len(op.deleted) == prev.pos
//         (each new delete sits immediately to the left of prev).
//       * Forward-Delete chain: op.pos == prev.pos and op.deleted's
//         bytes start where prev.deleted ended.
//     Same no-newline/no-tab restriction so paragraph boundaries
//     remain undo-able as their own step.
func canMergeEdit(prev, op editOp) bool {
	prevIns := len(prev.inserted) > 0 && len(prev.deleted) == 0
	prevDel := len(prev.deleted) > 0 && len(prev.inserted) == 0
	opIns := len(op.inserted) > 0 && len(op.deleted) == 0
	opDel := len(op.deleted) > 0 && len(op.inserted) == 0

	// Whitespace, newlines and tabs are natural undo boundaries —
	// users expect Ctrl+Z to undo "one word at a time", not the
	// entire paragraph.
	noBreak := func(b []byte) bool {
		for _, c := range b {
			if c == '\n' || c == '\t' || c == ' ' {
				return false
			}
		}
		return true
	}

	switch {
	case prevIns && opIns:
		if !noBreak(prev.inserted) || !noBreak(op.inserted) {
			return false
		}
		return op.pos == prev.pos+len(prev.inserted)
	case prevDel && opDel:
		if !noBreak(prev.deleted) || !noBreak(op.deleted) {
			return false
		}
		// Backspace: new delete sits to the left of prev.
		if op.pos+len(op.deleted) == prev.pos {
			return true
		}
		// Forward delete: new delete sits at the same anchor (since
		// prev's bytes are gone, the next forward delete still
		// reports its pos as prev.pos).
		if op.pos == prev.pos {
			return true
		}
	}
	return false
}

// mergeEditInto folds op into prev so both edits replay/unreplay as
// one. Caller has already verified compatibility via canMergeEdit.
func mergeEditInto(prev *editOp, op editOp) {
	switch {
	case len(prev.inserted) > 0 && len(op.inserted) > 0:
		// Insertion chain: append the new bytes to the running run
		// and slide selAfter forward.
		prev.inserted = append(prev.inserted, op.inserted...)
		prev.selAfter = op.selAfter
	case len(prev.deleted) > 0 && len(op.deleted) > 0:
		if op.pos+len(op.deleted) == prev.pos {
			// Backspace: the new bytes are *before* prev.deleted.
			prev.deleted = append(append([]byte{}, op.deleted...), prev.deleted...)
			prev.pos = op.pos
		} else {
			// Forward delete: new bytes follow prev.deleted.
			prev.deleted = append(prev.deleted, op.deleted...)
		}
		prev.selAfter = op.selAfter
	}
}

// Changed reports whether the buffer was mutated since the last call,
// then clears the dirty flag. Mirrors widget.Editor's ChangeEvent so
// callers can keep their existing per-frame poll loop.
func (v *RequestEditor) Changed() bool {
	d := v.dirty
	v.dirty = false
	return d
}

// Undo applies the inverse of the last edit. Returns true if anything
// was undone (false on empty stack). Pushes the undone op onto the
// redo stack so Redo can replay it.
func (v *RequestEditor) Undo() bool {
	if len(v.undoStack) == 0 {
		return false
	}
	op := v.undoStack[len(v.undoStack)-1]
	v.undoStack = v.undoStack[:len(v.undoStack)-1]
	v.suppressHistory = true
	// Reverse the op: remove `inserted`, splice `deleted` back in.
	if len(op.inserted) > 0 {
		v.DeleteRange(op.pos, op.pos+len(op.inserted))
	}
	if len(op.deleted) > 0 {
		v.Insert(op.pos, string(op.deleted))
	}
	v.suppressHistory = false
	v.selStart = op.selBefore
	v.selEnd = op.endBefore
	v.redoStack = append(v.redoStack, op)
	return true
}

// Redo replays the most recently undone op.
func (v *RequestEditor) Redo() bool {
	if len(v.redoStack) == 0 {
		return false
	}
	op := v.redoStack[len(v.redoStack)-1]
	v.redoStack = v.redoStack[:len(v.redoStack)-1]
	v.suppressHistory = true
	if len(op.deleted) > 0 {
		v.DeleteRange(op.pos, op.pos+len(op.deleted))
	}
	if len(op.inserted) > 0 {
		v.Insert(op.pos, string(op.inserted))
	}
	v.suppressHistory = false
	caret := op.selAfter
	v.selStart = caret
	v.selEnd = caret
	v.undoStack = append(v.undoStack, op)
	return true
}

// normSel returns selection bounds normalised so start <= end.
// Convenience for code that needs to act on the selection range as
// "this text was/will be replaced" without caring which end is the
// drag anchor.
func (v *RequestEditor) normSel() (int, int) {
	if v.selStart <= v.selEnd {
		return v.selStart, v.selEnd
	}
	return v.selEnd, v.selStart
}

// pushIMEState syncs gio's IME state with our buffer + selection so
// composing popups (Windows IME, macOS dead keys, mobile soft kbd
// suggestions) stay anchored to the right text. We send a Snippet
// covering a small window around the caret + a SelectionCmd locating
// the caret in rune indices. Both ranges are converted byte→rune at
// the boundary (gio's IME API is rune-indexed).
func (v *RequestEditor) pushIMEState(gtx layout.Context) {
	caretByte := v.selEnd
	caretRune := byteToRuneIdx(v.text, caretByte)
	selStartRune := byteToRuneIdx(v.text, v.selStart)
	selEndRune := caretRune

	gtx.Execute(key.SelectionCmd{
		Tag:   v,
		Range: key.Range{Start: selStartRune, End: selEndRune},
	})

	// Snippet covers a ±256-rune window around the caret. The IME may
	// use this to anticipate context (e.g. Asian-language word
	// segmentation) without us streaming the whole document each
	// frame.
	const window = 256
	startRune := caretRune - window
	if startRune < 0 {
		startRune = 0
	}
	endRune := caretRune + window
	totalRunes := utf8.RuneCount(v.text)
	if endRune > totalRunes {
		endRune = totalRunes
	}
	startByte := runeIdxToByte(v.text, startRune)
	endByte := runeIdxToByte(v.text, endRune)
	snip := key.Snippet{
		Range: key.Range{Start: startRune, End: endRune},
		Text:  string(v.text[startByte:endByte]),
	}
	if snip == v.imeSentSnippet {
		return
	}
	v.imeSentSnippet = snip
	gtx.Execute(key.SnippetCmd{Tag: v, Snippet: snip})
}

// shiftRanges adjusts selection / highlight byte offsets when the
// underlying buffer is edited at boundary `from` by `delta` bytes
// (positive on insertion, negative on deletion). Offsets at or past
// the boundary slide; offsets before the boundary are untouched.
// Selection that straddled a deletion gets clamped down to the
// deletion start so the caret doesn't end up dangling past EOF.
func (v *RequestEditor) shiftRanges(from, delta int) {
	adjust := func(off int) int {
		if off >= from {
			return off + delta
		}
		// On deletion: offset that fell inside the removed range
		// (i.e. between from+delta and from with delta<0) lands at the
		// new boundary.
		if delta < 0 && off > from+delta {
			return from + delta
		}
		return off
	}
	v.selStart = adjust(v.selStart)
	v.selEnd = adjust(v.selEnd)
	if v.highlightEnd > 0 {
		v.highlightStart = adjust(v.highlightStart)
		v.highlightEnd = adjust(v.highlightEnd)
	}
	if v.selStart < 0 {
		v.selStart = 0
	}
	if v.selEnd < 0 {
		v.selEnd = 0
	}
	if v.selStart > len(v.text) {
		v.selStart = len(v.text)
	}
	if v.selEnd > len(v.text) {
		v.selEnd = len(v.text)
	}
}

// Text returns a string copy of the buffer. Uses the same unsafe-looking
// but standard pattern as gio's editor.
func (v *RequestEditor) Text() string { return string(v.text) }

// Len returns the byte length of the buffer.
func (v *RequestEditor) Len() int { return len(v.text) }

// Selection returns the current highlight range. Kept for API parity.
func (v *RequestEditor) Selection() (int, int) {
	return v.highlightStart, v.highlightEnd
}

// SetCaret sets the highlight range and scrolls to bring it into view.
func (v *RequestEditor) SetCaret(start, end int) {
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
func (v *RequestEditor) SetScrollCaret(bool) {}

// GetScrollY returns the current vertical scroll position in pixels.
func (v *RequestEditor) GetScrollY() int { return v.scrollY }

// SetScrollY sets the vertical scroll position in pixels.
func (v *RequestEditor) SetScrollY(y int) {
	v.scrollY = y
	v.clampScroll()
}

// GetScrollBounds returns the scrollable extents. Y extent is a running
// estimate based on the last-rendered line height; it stabilises once
// a frame has run.
func (v *RequestEditor) GetScrollBounds() image.Rectangle {
	if v.lastLineHeight == 0 {
		return image.Rectangle{}
	}
	totalH := len(v.lineStarts) * v.lastLineHeight
	return image.Rectangle{Max: image.Point{Y: totalH}}
}

func (v *RequestEditor) clampScroll() {
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

func (v *RequestEditor) scrollToByteOffset(off int) {
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
func (v *RequestEditor) lineForByteOffset(off int) int {
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

func (v *RequestEditor) rebuildLineStartsFrom(startIdx int) {
	// Truncate existing entries past startIdx, then re-scan.
	for len(v.lineStarts) > 0 && v.lineStarts[len(v.lineStarts)-1] > startIdx {
		v.lineStarts = v.lineStarts[:len(v.lineStarts)-1]
	}
	if len(v.lineStarts) == 0 {
		v.lineStarts = append(v.lineStarts, 0)
	}
	v.scanChunks(v.lineStarts[len(v.lineStarts)-1])
}

func (v *RequestEditor) appendLineStartsFrom(startIdx int) {
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
func (v *RequestEditor) scanChunks(from int) {
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

// RequestEditorStyle renders a RequestEditor. Create one per Layout
// call — it's a value type holding only refs and style knobs.
type RequestEditorStyle struct {
	Viewer         *RequestEditor
	Shaper         *text.Shaper
	Font           font.Font
	TextSize       unit.Sp
	Color          color.NRGBA
	HighlightColor color.NRGBA // search match background
	SelectionColor color.NRGBA // mouse selection background
	Wrap           bool
	// ReadOnly suppresses the caret and ignores keyboard input that
	// would mutate the buffer (text input, Backspace, Delete, paste,
	// cut, Tab, Enter). Selection and copy still work.
	ReadOnly bool
	// Padding is applied inside the viewer's clip — text and selection
	// rectangles shift by Padding while gestures and scroll bounds still
	// cover the full clipped region. Same value applies to wrap and
	// non-wrap modes so users see consistent breathing room.
	Padding unit.Dp
	// Env is the active environment's resolved variable map. When
	// non-nil, the editor scans visible chunks for {{var}} patterns
	// and paints colorVarFound / colorVarMissing rectangles behind
	// them — same colour scheme as TextField in widgets.go. Pass nil
	// to disable highlighting (cheap; the scan is skipped entirely).
	Env map[string]string

	// Lang drives syntax coloring. LangPlain (zero) disables it
	// entirely — same single-Color path as before. Buffers larger than
	// requestEditorTokenizeMaxBytes also fall back to plain regardless.
	Lang syntax.Lang

	// Syntax is the per-token-kind palette + bracket cycle. Used only
	// when Lang != LangPlain.
	Syntax       syntaxPalette
	BracketCycle bool
}

func (s RequestEditorStyle) Layout(gtx layout.Context) layout.Dimensions {
	v := s.Viewer

	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 {
		return layout.Dimensions{Size: size}
	}

	// Refresh the document-wide token stream when language switches
	// or when the buffer's length changes (a cheap heuristic for
	// "user touched the body"). Skip tokenization for huge buffers
	// to keep typing latency bounded.
	tokenizing := s.Lang != syntax.LangPlain && len(v.text) <= requestEditorTokenizeMaxBytes
	if tokenizing {
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
	// Drain hover events on the viewer's tag — gio only applies the
	// cursor hint where a pointer target is actively listening, so
	// without this Move/Enter/Leave subscription the I-beam stays
	// dormant even with CursorText.Add registered.
	for {
		_, ok := gtx.Event(pointer.Filter{Target: v, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
		if !ok {
			break
		}
	}
	// Input hint so soft keyboards open in plain-text mode (matters on
	// Android / mobile; desktop ignores it).
	key.InputHintOp{Tag: v, Hint: key.HintText}.Add(gtx.Ops)

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
		v.blinkStart = gtx.Now
		// Push the new selection back to gio so the next EditEvent's
		// Range matches the user-visible caret. Without this, gio
		// keeps reporting the stale range from the last SelectionCmd
		// (typically the end-of-buffer baseline pushed on focus),
		// which makes the very first keystroke insert at the wrong
		// position and the caret "jump to end of line".
		v.pushIMEState(gtx)
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
			// Same reason as in the Click handler: keep gio in sync
			// with the user-visible selection so the next keystroke
			// inserts at the right place.
			v.pushIMEState(gtx)
		}
	}

	// 3a. Text input + IME. Subscribing the editor's tag to FocusFilter +
	//     transfer.TargetFilter routes EditEvent / SnippetEvent /
	//     SelectionEvent / FocusEvent / DataEvent to us automatically
	//     once the widget is focused. EditEvent ranges are rune indices
	//     (gio's IME API), but our internal selection is byte-based —
	//     so we convert at the boundary using runeIdxToByte /
	//     byteToRuneIdx. After every textual mutation we push a
	//     SnippetCmd + SelectionCmd back to gio so the IME stays in
	//     sync with the buffer.
	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: v},
			transfer.TargetFilter{Target: v, Type: "application/text"},
			key.Filter{Focus: v, Name: key.NameDeleteBackward, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NameDeleteForward, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NameReturn, Optional: key.ModShift},
			key.Filter{Focus: v, Name: key.NameEnter, Optional: key.ModShift},
			key.Filter{Focus: v, Name: key.NameTab, Optional: key.ModShift},
			key.Filter{Focus: v, Name: "V", Required: key.ModShortcut},
			key.Filter{Focus: v, Name: "X", Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		// Any handled event resets blink so the caret stays solid for a
		// beat after the user does something. The render loop reads
		// blinkStart and schedules the next invalidate accordingly.
		v.blinkStart = gtx.Now
		switch ke := ev.(type) {
		case key.FocusEvent:
			if ke.Focus {
				gtx.Execute(key.SoftKeyboardCmd{Show: true})
				v.pushIMEState(gtx)
			} else {
				v.imeStart, v.imeEnd = 0, 0
			}
		case key.EditEvent:
			// gio sends ke.Range in rune indices, derived from the IME
			// state we last pushed. In wrap mode that state can lag the
			// caret by one position when the user clicks-then-types in
			// quick succession (the SelectionCmd from the click hasn't
			// been processed yet, so gio still uses the old selection
			// and the inserted char lands one column to the left of the
			// visible caret). v.selStart / v.selEnd are updated
			// synchronously by the click handler, so insert there
			// directly. IME composition (where ke.Range diverges from
			// the active selection) is bracketed by SnippetEvent and
			// uses imeStart/imeEnd, which we still respect on the
			// next render.
			start, end := v.normSel()
			v.Replace(start, end, ke.Text)
			caret := start + len(ke.Text)
			v.selStart = caret
			v.selEnd = caret
			v.imeStart, v.imeEnd = 0, 0
			v.ensureCaretVisible()
			v.pushIMEState(gtx)
		case key.SnippetEvent:
			// IME composing region update. Stored in byte offsets so
			// our render pass can paint an underline; gio reports it
			// in rune indices.
			v.imeStart = runeIdxToByte(v.text, ke.Start)
			v.imeEnd = runeIdxToByte(v.text, ke.End)
		case key.SelectionEvent:
			startB := runeIdxToByte(v.text, ke.Start)
			endB := runeIdxToByte(v.text, ke.End)
			v.selStart = startB
			v.selEnd = endB
			v.ensureCaretVisible()
		case transfer.DataEvent:
			// Paste payload arrives via the transfer pipeline once
			// clipboard.ReadCmd has resolved. Insert at caret and
			// collapse the selection onto the end of the inserted
			// text.
			rd := ke.Open()
			data, err := io.ReadAll(rd)
			rd.Close()
			if err == nil && len(data) > 0 {
				start, end := v.normSel()
				v.Replace(start, end, string(data))
				caret := start + len(data)
				v.selStart = caret
				v.selEnd = caret
				v.ensureCaretVisible()
				v.pushIMEState(gtx)
			}
		case key.Event:
			if ke.State != key.Press {
				continue
			}
			switch ke.Name {
			case key.NameDeleteBackward:
				if v.selStart != v.selEnd {
					start, end := v.normSel()
					v.DeleteRange(start, end)
					v.selStart = start
					v.selEnd = start
				} else if v.selEnd > 0 {
					prev := v.charLeft(v.selEnd)
					v.DeleteRange(prev, v.selEnd)
					v.selStart = prev
					v.selEnd = prev
				}
				v.ensureCaretVisible()
				v.pushIMEState(gtx)
			case key.NameDeleteForward:
				if v.selStart != v.selEnd {
					start, end := v.normSel()
					v.DeleteRange(start, end)
					v.selStart = start
					v.selEnd = start
				} else if v.selEnd < len(v.text) {
					next := v.charRight(v.selEnd)
					v.DeleteRange(v.selEnd, next)
				}
				v.ensureCaretVisible()
				v.pushIMEState(gtx)
			case key.NameReturn, key.NameEnter:
				start, end := v.normSel()
				v.Replace(start, end, "\n")
				caret := start + 1
				v.selStart = caret
				v.selEnd = caret
				v.ensureCaretVisible()
				v.pushIMEState(gtx)
			case key.NameTab:
				start, end := v.normSel()
				v.Replace(start, end, "\t")
				caret := start + 1
				v.selStart = caret
				v.selEnd = caret
				v.ensureCaretVisible()
				v.pushIMEState(gtx)
			case "V":
				gtx.Execute(clipboard.ReadCmd{Tag: v})
			case "X":
				if sel := v.SelectedText(); sel != "" {
					gtx.Execute(clipboard.WriteCmd{
						Type: "application/text",
						Data: io.NopCloser(strings.NewReader(sel)),
					})
					start, end := v.normSel()
					v.DeleteRange(start, end)
					v.selStart = start
					v.selEnd = start
					v.ensureCaretVisible()
					v.pushIMEState(gtx)
				}
			}
		}
		hasSel = v.selStart != v.selEnd
	}

	// 3b. Keyboard navigation + shortcuts (no buffer mutation).
	//     Modifier-shortcuts (Ctrl+A/Ctrl+C/Ctrl+Home/End/Ctrl+arrow) and
	//     bare navigation (arrows, Home, End, PageUp/PageDown) — Shift
	//     extends the existing selection; bare keypress collapses it
	//     onto the new caret position.
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: v, Name: "A", Required: key.ModShortcut},
			key.Filter{Focus: v, Name: "C", Required: key.ModShortcut},
			key.Filter{Focus: v, Name: "Z", Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Focus: v, Name: "Y", Required: key.ModShortcut},
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
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		v.blinkStart = gtx.Now
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
		case "Z":
			if ke.Modifiers.Contain(key.ModShift) {
				if v.Redo() {
					v.ensureCaretVisible()
					v.pushIMEState(gtx)
				}
			} else {
				if v.Undo() {
					v.ensureCaretVisible()
					v.pushIMEState(gtx)
				}
			}
		case "Y":
			if v.Redo() {
				v.ensureCaretVisible()
				v.pushIMEState(gtx)
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
		// Mirror the new caret/selection back to gio so the next
		// keystroke routed through EditEvent inserts where the user
		// just navigated to (otherwise gio keeps the stale range
		// from the last SelectionCmd and inserts off-position).
		v.pushIMEState(gtx)
	}

	// Caret blink: hold solid for one period after the last move/edit
	// (so the user can track it), then alternate every 500ms. Schedule
	// the next invalidate at the toggle boundary so the cursor pulses
	// without forcing a continuous redraw.
	const blinkPeriod = 500 * time.Millisecond
	const blinkSolid = blinkPeriod
	caretFocused := gtx.Focused(v) && v.selStart == v.selEnd && !s.ReadOnly
	caretShow := caretFocused
	if caretFocused {
		elapsed := gtx.Now.Sub(v.blinkStart)
		if elapsed < blinkSolid {
			gtx.Execute(op.InvalidateCmd{At: v.blinkStart.Add(blinkSolid)})
		} else {
			rem := elapsed - blinkSolid
			phase := rem / blinkPeriod
			caretShow = phase%2 == 0
			next := v.blinkStart.Add(blinkSolid + (phase+1)*blinkPeriod)
			gtx.Execute(op.InvalidateCmd{At: next})
		}
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

		// Caret: a 1-px vertical bar at the cursor position. Only drawn
		// when this chunk contains the cursor and we're in the visible
		// half of the blink cycle.
		if caretShow && v.selEnd >= start && v.selEnd <= end {
			v.paintCaret(gtx, start, end, yOff, charAdv, exactLineH, s.Wrap, innerW, s.Color)
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

		// {{var}} highlighting — scans only the bytes of this visible
		// chunk so the per-frame cost is O(visible chars) instead of
		// O(whole document). Disabled past requestEditorVarScanCutoff
		// because at that scale the body is almost always
		// machine-generated and per-var feedback isn't worth the
		// scan + extra paint ops.
		if len(v.text) <= requestEditorVarScanCutoff {
			v.paintVarHighlights(gtx, start, end, yOff, charAdv, exactLineH, s.Wrap, innerW, s.Env)
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
		if tokenizing && len(v.tokens) > 0 {
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
func (v *RequestEditor) coordToByteOffset(
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
func (v *RequestEditor) estimateChunkHeight(line, lineHeight int, advance fixed.Int26_6, viewportW int, wrap bool) int {
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

// firstChunkAtFn returns the chunk index whose vertical range contains
// pixel offset y from the document top, plus the accumulated y where
// that chunk begins. Unmeasured chunks contribute estimateChunkHeight
// each (a per-chunk byte-length × chars_per_line projection).
func (v *RequestEditor) firstChunkAtFn(y, lineH int, advance fixed.Int26_6, viewportW int, wrap bool) (int, int) {
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
func (v *RequestEditor) lineBounds(n int) (int, int) {
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
func (v *RequestEditor) wordBoundsAt(byteOff int) (int, int) {
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
func (v *RequestEditor) sourceLineBoundsAt(byteOff int) (int, int) {
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
func (v *RequestEditor) SelectAll() {
	v.selStart = 0
	v.selEnd = len(v.text)
	v.dragActive = false
}

// moveCaret applies a byte-offset move to the caret, with optional
// selection extension. extend=false collapses any selection onto the
// new caret position; extend=true keeps selStart as the anchor and
// follows with selEnd, matching native editor convention.
func (v *RequestEditor) moveCaret(newPos int, extend bool) {
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
func (v *RequestEditor) charLeft(off int) int {
	if off <= 0 {
		return 0
	}
	_, sz := utf8.DecodeLastRune(v.text[:off])
	return off - sz
}

func (v *RequestEditor) charRight(off int) int {
	if off >= len(v.text) {
		return len(v.text)
	}
	_, sz := utf8.DecodeRune(v.text[off:])
	return off + sz
}

// wordLeft / wordRight: move past adjacent separators, then over a
// run of word chars. Same separator definition as double-click word
// selection (isSeparator).
func (v *RequestEditor) wordLeft(off int) int {
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

func (v *RequestEditor) wordRight(off int) int {
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
func (v *RequestEditor) columnAt(off int) int {
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
func (v *RequestEditor) offsetAtColumn(lineStart, col int) int {
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
func (v *RequestEditor) lineUp(off, col int) int {
	lineStart, _ := v.sourceLineBoundsAt(off)
	if lineStart == 0 {
		return 0
	}
	prevLineStart, _ := v.sourceLineBoundsAt(lineStart - 1)
	return v.offsetAtColumn(prevLineStart, col)
}

func (v *RequestEditor) lineDown(off, col int) int {
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
func (v *RequestEditor) visualColumnAt(off, cpl int) int {
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
// (lineStarts) and within a chunk by sub-line of width cpl.
func (v *RequestEditor) wrapLineMove(off, prefVisualCol, cpl, dir int) int {
	if cpl < 1 {
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
	subLine := inChunkRune / cpl

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
func (v *RequestEditor) ensureCaretVisible() {
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
// requestVarClickTag is the per-{{var}} pointer target for hover /
// click. start is the byte offset in the buffer where `{{` begins —
// it's the stable identity gio sees, so that as Insert/Delete shift
// the rest of the buffer the same chip keeps generating events
// against the same tag instance only while its byte position is
// unchanged within a frame.
type requestVarClickTag struct {
	ed    *RequestEditor
	start int
}

// paintVarHighlights scans [chunkStart, chunkEnd) for `{{name}}`
// patterns and paints a coloured rect behind each one (green when
// present in env, red when missing — matches TextField). Also
// registers a per-chip pointer target so hover shows the value
// tooltip and click opens the var-edit popup, matching TextField's
// behaviour. Renders in the same coordinate space as the chunk's
// text (after the chunk op.Offset and scrollX shift).
//
// Wrap mode: a `{{var}}` that would straddle a wrapped sub-line
// boundary is split visually by gio's WrapGraphemes policy. We split
// the highlight to match — one rect per affected sub-line.
func (v *RequestEditor) paintVarHighlights(
	gtx layout.Context,
	chunkStart, chunkEnd int,
	yOff int,
	advance fixed.Int26_6,
	exactLineH int,
	wrap bool,
	viewportW int,
	env map[string]string,
) {
	if advance <= 0 || chunkEnd <= chunkStart {
		return
	}
	chunkText := v.text[chunkStart:chunkEnd]
	if !bytesContainsTwoBraces(chunkText) {
		return
	}
	cornerR := gtx.Dp(unit.Dp(3))
	padY := gtx.Dp(unit.Dp(2))
	cpl := 0
	if wrap {
		cpl = charsPerLineFor(viewportW, advance)
	}

	idx := 0
	for idx < len(chunkText) {
		s := bytesIndex(chunkText[idx:], "{{")
		if s == -1 {
			break
		}
		s += idx
		e := bytesIndex(chunkText[s+2:], "}}")
		if e == -1 {
			break
		}
		e = s + 2 + e + 2
		name := strings.TrimSpace(string(chunkText[s+2 : e-2]))
		bgColor := colorVarMissing
		if _, ok := env[name]; ok && len(env) > 0 {
			bgColor = colorVarFound
		}

		startRune := byteToRuneIdx(chunkText, s)
		endRune := byteToRuneIdx(chunkText, e)

		// Same fixed-point gotcha as paintCaret: multiply Int26_6
		// advance by a plain int rune column, then Round().
		colToPx := func(c int) int {
			return (advance * fixed.Int26_6(c)).Round()
		}
		// Compute the chip's primary rect (first sub-line in wrap
		// mode) and use that as the pointer-event area. Multi-line
		// wrap chips get the highlight on every sub-line but only
		// the first sub-line is interactive — same trade-off the
		// TextField overlay makes (the popup target is one rect).
		var hitRect image.Rectangle
		if !wrap {
			x1 := colToPx(startRune) - v.scrollX
			x2 := colToPx(endRune) - v.scrollX
			hitRect = image.Rect(x1, yOff-padY, x2, yOff+exactLineH+padY)
			paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(hitRect, cornerR).Op(gtx.Ops))
		} else {
			if cpl < 1 {
				cpl = 1
			}
			startLine := startRune / cpl
			endLine := (endRune - 1) / cpl
			for ln := startLine; ln <= endLine; ln++ {
				colStart := 0
				colEnd := cpl
				if ln == startLine {
					colStart = startRune % cpl
				}
				if ln == endLine {
					colEnd = ((endRune - 1) % cpl) + 1
				}
				x1 := colToPx(colStart)
				x2 := colToPx(colEnd)
				y := yOff + ln*exactLineH
				rect := image.Rect(x1, y-padY, x2, y+exactLineH+padY)
				paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(rect, cornerR).Op(gtx.Ops))
				if ln == startLine {
					hitRect = rect
				}
			}
		}

		// Register a pointer target on the chip rect so app.go's
		// var-popup machinery picks up press / hover / leave. The
		// tag's `start` is the chip's byte offset within the buffer
		// (not within the chunk) so identity stays stable across
		// chunk boundaries.
		chipStart := chunkStart + s
		chipEnd := chunkStart + e
		tag := requestVarClickTag{ed: v, start: chipStart}
		stack := clip.Rect(hitRect).Push(gtx.Ops)
		event.Op(gtx.Ops, tag)
		v.processVarChipEvents(gtx, tag, hitRect, name, chipStart, chipEnd)
		stack.Pop()

		idx = e
	}
}

// processVarChipEvents drains pointer events for a var chip's tag
// and routes them to the global hover / click state that app.go's
// popup layer reads. Mirrors widgets.go's varClickTag handling so
// the same popup fires whether the chip lives in a TextField or in
// the request body.
func (v *RequestEditor) processVarChipEvents(
	gtx layout.Context,
	tag requestVarClickTag,
	rect image.Rectangle,
	name string,
	chipStart, chipEnd int,
) {
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: tag,
			Kinds:  pointer.Press | pointer.Enter | pointer.Leave,
		})
		if !ok {
			return
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch pe.Kind {
		case pointer.Press:
			if !pe.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			// pe.Position is in handler-local coords (origin =
			// rect.Min); GlobalPointerPos is window coords. Their
			// delta gives the window-space origin of the chip's
			// rect. We use the bottom-left so the popup sits flush
			// under the chip.
			originX := GlobalPointerPos.X - pe.Position.X
			originY := GlobalPointerPos.Y - pe.Position.Y
			GlobalVarClick = &VarHoverState{
				Name:   name,
				Pos:    f32.Pt(originX, originY+float32(rect.Dy())),
				Editor: v,
				Range:  struct{ Start, End int }{chipStart, chipEnd},
			}
		case pointer.Enter:
			originX := GlobalPointerPos.X - pe.Position.X
			originY := GlobalPointerPos.Y - pe.Position.Y
			GlobalVarHover = &VarHoverState{
				Name:   name,
				Pos:    f32.Pt(originX, originY+float32(rect.Dy())),
				Editor: v,
				Range:  struct{ Start, End int }{chipStart, chipEnd},
			}
		case pointer.Leave:
			if GlobalVarHover != nil &&
				GlobalVarHover.Editor == v &&
				GlobalVarHover.Range.Start == chipStart {
				GlobalVarHover = nil
			}
		}
	}
}

// bytesIndex is strings.Index for []byte without allocating a string.
func bytesIndex(b []byte, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	return strings.Index(string(b), sub)
}

func bytesContainsTwoBraces(b []byte) bool {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == '{' && b[i+1] == '{' {
			return true
		}
	}
	return false
}

// paintCaret draws a 1-pixel vertical bar at v.selEnd inside the
// chunk [chunkStart, chunkEnd). Translates byte offset → rune column
// → pixel x using the same fixed-point advance the render path uses,
// so the bar lines up with the gap between glyphs even on
// non-integer advances. In wrap mode the rune column wraps every
// charsPerLine characters (matching our chars_per_line math), and
// the y is offset by the wrapped sub-line.
func (v *RequestEditor) paintCaret(
	gtx layout.Context,
	chunkStart, chunkEnd int,
	yOff int,
	advance fixed.Int26_6,
	exactLineH int,
	wrap bool,
	viewportW int,
	col color.NRGBA,
) {
	if advance <= 0 {
		return
	}
	caretByte := v.selEnd - chunkStart
	if caretByte < 0 {
		caretByte = 0
	}
	chunkText := v.text[chunkStart:chunkEnd]
	caretRune := byteToRuneIdx(chunkText, caretByte)

	// Use the same colToPx pattern as paintHighlight: multiply
	// advance (Int26_6) by a plain int rune column, then Round() back
	// to pixels. fixed.I(col) * advance is wrong here — it shifts col
	// into Int26_6, so the multiplication promotes to Int52_12 and
	// .Floor() returns garbage in pixels (~64× too large).
	colToPx := func(c int) int {
		return (advance * fixed.Int26_6(c)).Round()
	}
	var x, y int
	if !wrap {
		x = colToPx(caretRune) - v.scrollX
		y = yOff
	} else {
		cpl := charsPerLineFor(viewportW, advance)
		if cpl < 1 {
			cpl = 1
		}
		subLine := caretRune / cpl
		colIdx := caretRune % cpl
		// End-of-chunk-text on a wrap boundary: the rune index is
		// exactly the start of the next visual line, but there's no
		// next-line glyph to anchor it to (the chunk ended). Render
		// at the trailing edge of the previous visual line instead so
		// the caret sits visually after the last printable char,
		// which matches what a click at end-of-line just produced.
		// Without this, a click after the last char of a fully-
		// packed line drops the caret onto a phantom next line, and
		// the next typed char appears below+left of where the user
		// clicked.
		chunkRunes := utf8.RuneCount(chunkText)
		if caretRune > 0 && caretRune == chunkRunes && colIdx == 0 {
			subLine = (caretRune - 1) / cpl
			colIdx = cpl
		}
		x = colToPx(colIdx)
		y = yOff + subLine*exactLineH
	}
	caretW := gtx.Dp(unit.Dp(1))
	if caretW < 1 {
		caretW = 1
	}
	rect := image.Rect(x, y, x+caretW, y+exactLineH)
	paint.FillShape(gtx.Ops, col, clip.Rect(rect).Op())
}

// Used by both search highlight and mouse selection — the same
// machinery, just different colours and ranges.
func (v *RequestEditor) paintHighlight(
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
