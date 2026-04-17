# go-tracto

Desktop API client similar to Postman/Insomnia, written in Go on top of the Gio immediate-mode GUI framework. Single-binary, no Electron, no web runtime. The module path is `tracto` (not `go-tracto`) — imports use `tracto/internal/...`.

## Stack

- **Go 1.25.5**
- **UI**: `github.com/nanorele/gio` + `github.com/nanorele/gio-x` — a **custom fork** of Gio. Do not assume upstream Gio APIs are available; the fork adds methods like `widget.Editor.GetScrollX/Y`, `SetScrollX/Y`, `GetScrollBounds`, `Len`, `Insert`, `SetCaret`. If you need a feature not in stock Gio, it might already exist on the fork — check `go.sum` / the editor struct before rewriting.
- **Font**: embedded `internal/ui/assets/fonts/NotoColorEmoji.ttf` + gofont collection. Mono font referenced by name `"Ubuntu Mono"` (must be installed on the system; there is no embedded mono fallback).
- **Icons**: `golang.org/x/exp/shiny/materialdesign/icons`.
- **CGO is required** (Gio's OpenGL/Vulkan backends). On Windows use MSYS2 mingw-w64 toolchain; on Linux install GCC.

## Layout

```
cmd/main.go                  Entry point. Spawns UI goroutine, calls app.Main().
internal/ui/
  app.go        (~85 KB)     AppUI shell: title bar, sidebar, tab bar, popups, state persistence, global event routing.
  tab.go        (~57 KB)     RequestTab: method/URL/body/headers panes, response pane, HTTP execution, streaming, search, JSON pretty-print.
  widgets.go    (~16 KB)     Shared widgets: TextField, TextFieldOverlay (with {{var}} highlighting), SquareBtn, menuOption, word-motion shortcuts, text measurement caches.
  collection.go              Postman-compatible collection parser + CollectionNode tree + Ext* JSON DTOs.
  environment.go             Environment parser + EnvVarRow editor state.
  state.go                   AppState JSON persistence. Config dir: os.UserConfigDir()/tracto/{state.json, collections/*.json, environments/*.json}.
  colors.go                  Full dark palette + HTTP method color mapping (getMethodColor).
  assets/fonts/              Embedded emoji font.
internal/utils/text.go       SanitizeText / SanitizeBytes (strips control chars, normalizes CRLF, keeps valid UTF-8) + StripJSONComments.
```

Everything under `internal/` — not meant for reuse outside this binary.

## Core types and their relationships

- **`AppUI`** (app.go): root state. Owns `Tabs []*RequestTab`, `Collections []*CollectionUI`, `Environments []*EnvironmentUI`, sidebar geometry, drag state, popups (tab context menu, variable edit popup, var hover tooltip), `activeEnvVars map[string]string` derived from `ActiveEnvID`.
- **`RequestTab`** (tab.go): one open request. Holds URLInput/ReqEditor/RespEditor (`widget.Editor`), `Headers []*HeaderItem`, HTTP client state (`cancelFn`, `requestID`, `responseChan`, `appendChan`), preview/streaming state (`respFile` temp path, `respSize`, `previewLoaded`, `respIsJSON`), split ratios, search state, optional link to a collection node via `LinkedNode`.
- **`CollectionNode`** (collection.go): tree node. Either a folder (`IsFolder`, `Children`) or a request (`Request *ParsedRequest`). Carries its own `widget.Clickable`/menu state and a `NameEditor` for in-place rename. Backlink to parent and owning `ParsedCollection`.
- **`ParsedEnvironment`** / **`EnvironmentUI`**: separation of data (Name, Vars) from editor UI state (Rows, editors). `EnvironmentUI.initEditor()` must be called before editing.
- **`HeaderItem`**: has `IsGenerated` flag. Auto-generated headers (User-Agent, Content-Type) are inserted/refreshed by `updateSystemHeaders()` every edit; the moment the user types over them they flip to user-owned (`IsGenerated=false`) and are never auto-reset.

## Gio-specific patterns used here

Read this before touching UI code.

- **Immediate mode**: `layout()` runs every frame. State mutations happen *during* layout by polling `widget.Clickable.Clicked(gtx)`, `widget.Editor.Update(gtx)`, `gesture.Drag.Update(...)`. Never store callbacks — check button state each frame.
- **Event loop**: `AppUI.Run()` owns the `app.Window.Event()` loop. `FrameEvent` triggers a render; `DestroyEvent` saves state; `ConfigEvent` tracks maximize; `transfer.DataEvent` handles drag-drop import.
- **Deferred rendering for popups/menus**: `op.Record` → `op.Defer(gtx.Ops, macro.Stop())`. Used for method dropdown, send menu, sidebar context menus, var tooltip. Do not draw popups inline — they will clip.
- **Global pointer tracking**: `ui.LastPointerPos` is refreshed each frame via a `pointer.Filter{Target: ui, ...}` — used by `GlobalPointerPos` for variable-chip hover detection in nested editor widgets that don't get their own hover events.
- **Invalidations**: call `ui.Window.Invalidate()` from background goroutines after mutating shared state (response chunk appended, collection loaded, etc.). Otherwise the frame won't redraw.
- **Width freezing during drag** (`LastReqWidth`, `LastURLWidth`, `LastRespWidth`, `pendingReqWidth`): when the split drag is active, the editors keep their last width to avoid expensive re-layout; after drag ends there's a 300 ms debounce before accepting the new width. Respect this when touching editor sizing.
- **Focus / rename flow**: `gtx.Execute(key.FocusCmd{Tag: &editor})` to focus programmatically. Rename commits on blur (`RenamingFocused` sentinel), Enter (`widget.SubmitEvent`), or Ctrl+S; Esc cancels.

## State & persistence

- Config path: `os.UserConfigDir()/tracto/`.
- `state.json`: tabs (title, method, URL, body, headers, split ratios, ReqWrap, linked collection+node path), active tab, active env ID, sidebar sizes.
- `collections/<hex16>.json`: Postman-v2-ish format; `buildExtItems` serializes the node tree back to `ExtCollection` on save. IDs are generated on import with `crypto/rand`.
- `environments/<hex16>.json`: `{name, values:[{key,value,enabled}]}`.
- Save model: `saveState()` sets `saveNeeded=true`; `flushSaveState()` at end of frame writes **async** in a goroutine. `saveStateSync()` is used only on `DestroyEvent`.
- Tabs link to collection nodes by `{CollectionID, NodePath []int}`; `relinkTabs()` reattaches once async-loaded collections arrive on `ColLoadedChan`.

## Request execution pipeline

1. `prepareRequest(env)`: trim URL, expand `{{var}}` via `processTemplate`, ensure scheme (default http://), strip JSON `//` comments from body (only if result is still valid JSON), apply system headers.
2. `executeRequest` / `executeRequestToFile`:
   - Bumps `requestID` — in-flight responses with stale IDs are discarded.
   - Streams `resp.Body` to a `tmpFile` (`os.CreateTemp("", "tracto-resp-*.tmp")`) **always**, even in preview mode. This is the backing store for "Load more".
   - Live preview: first `maxStreamPreview = 512 KB` is pushed through `appendChan` to the response editor as it arrives, throttled by `time.Since(lastUpdate) > 250 ms` invalidations.
   - After download, `loadPreviewFromFile` reads up to `previewBatchSize = 15 MB`, detects JSON from the first 64 bytes (`looksLikeJSON`), pretty-prints via the custom `indentJSON` (hand-rolled, avoids `encoding/json`'s allocation cost), and populates the editor.
   - `LoadMoreBtn` reads further batches from `respFile` starting at `previewLoaded`.
- Cancel path: `cancelFn` set on the context; `t.cancelRequest()` calls it; `responseChan` consumer checks `requestID` before accepting.
- Goroutine→UI communication is via buffered channels (`responseChan` cap 1, `appendChan` cap 128, `FileSaveChan` cap 1) drained inside `layout()` each frame.

## Variable templating

- Syntax: `{{name}}` in URL, body, header key/value.
- Resolution: `processTemplate` against `activeEnvVars` (the enabled vars of the active environment). Unresolved names are left literal.
- Visual: `TextFieldOverlay` / `TextField` scan the text for `{{...}}` every frame, measure each span, and draw a colored rect behind it — blue (`colorVarFound`) if defined, red (`colorVarMissing`) otherwise.
- Interaction: hovering a chip sets `GlobalVarHover` → tooltip in `layoutApp`. Clicking sets `GlobalVarClick` → edit popup that can inline-replace the var with another one (splices `{{other}}` into the source editor text).

## Things that will bite you if you ignore them

- **Don't call `material.Editor(...)` without setting width constraints manually** — the free editor layout path goes through `ReqListH` / `RespListH` horizontal lists when wrap is off; wrap-on path pins width explicitly so the shaper doesn't re-run on every pixel change.
- **Auto-save collections after structural edits**: rename/dup/delete/reparent → `SaveCollectionToFile(n.Collection)`. `saveState()` alone won't persist tree changes.
- **Dirty tracking** (`RequestTab.IsDirty`): computed lazily via `dirtyCheckNeeded`; cheap bail-outs by comparing `Len()` before `Text()`. If you add a new editable field, feed into `checkDirty()` and set `dirtyCheckNeeded=true` on its ChangeEvent.
- **`getCleanTitle()` is cached** by source equality — mutate `t.Title` and the cache invalidates itself. Don't read `Title` directly for display.
- **Sanitize all imported text** via `utils.SanitizeText` / `SanitizeBytes`. Collection JSON from the wild contains BOMs, RTL overrides, and control chars that break Gio's shaper.
- **Frameless window**: `app.Decorated(false)`. We draw our own title bar (`layoutTitleBtn`) and implement maximize/minimize/close + drag via `system.ActionCmd`-style clicks. Don't rely on OS chrome.
- **`widget.Editor.Submit = true`** enables Enter-to-submit (`widget.SubmitEvent`); used on URL field and rename editors. Multi-line editors must leave it off.
- **Content-Type auto-detection** is first-byte only: `{` or `[` → `application/json`, else `text/plain`. Overridden the moment the user edits the generated header.

## HTTP, timeouts, pooling

- Single shared `http.Client{}` (no explicit timeout — per-request cancellation is done via context).
- Stream read buffer is pooled: `streamBufPool` (256 KB) and `previewBufPool` (15 MB).
- `indentTable[64]` pre-renders `"\n" + 2*n spaces` for the JSON pretty-printer.

## Keyboard shortcuts (handled in `layoutContent`)

- `Ctrl+Enter`: send current request
- `Ctrl+S`: save current tab → linked collection node
- `Ctrl+W`: close current tab
- `Ctrl+F`: toggle search panel in response
- `Ctrl+←/→` inside any editor: word-by-word motion (`moveWord` in widgets.go, separators are unicode whitespace + `.,:;!?-()[]{}`)
- `Esc`: closes open popups / cancels rename

## Build & run

```
# Linux / macOS / Windows-with-CGO
go run ./...                      # dev
go build -o bin/tracto ./cmd      # plain build
make run / make build             # Windows only; requires gcc on PATH
```

Release build (Windows, from `readme.md`):
```
set GOAMD64=v3 && go build -gcflags="-B" -trimpath -ldflags="-s -w -H=windowsgui" -o bin\tracto.exe cmd\main.go && upx --best --lzma bin\tracto.exe
```

`-H=windowsgui` suppresses the console window. UPX+LZMA shrinks the binary (CGO+Gio makes it ~28 MB unpacked).

## Code style conventions observed in this repo

- No comments except where behavior is non-obvious (very few present).
- Package-level `var` blocks for icons initialized in `init()`.
- Error handling is pragmatic: parse/IO errors are frequently swallowed (collections skip on parse failure, save failures are silent). Don't add panics or noisy logs — the UI has no log pane.
- Channel-based async; no mutexes on UI state (single goroutine owns AppUI, background goroutines push over channels and call `Invalidate`).
- `atomic.Int64` on `downloadedBytes` — the only field read cross-goroutine without a channel.
- Buffers reused via `[:0]` slicing (`ui.VisibleCols`, `t.visibleHeadersBuf`, `ui.tabInfoBuf`) to avoid per-frame allocations in hot paths.

## When adding features

- New persisted field on a tab → add to `TabState` (state.go), read in `loadState()`, write in `buildStateSnapshot()`.
- New UI color → add to `colors.go`, keep the naming scheme (`colorBg*`, `colorFg*`, `colorAccent*`).
- New icon → declare at top of the file that uses it, init in that file's `init()` via `widget.NewIcon(icons.*)`.
- New background task writing to a tab → use a buffered channel drained in `RequestTab.layout` or `AppUI.Run`'s select, and call `win.Invalidate()`.
- New editor → `SingleLine`+`Submit` for one-liners; always feed through `TextField` / `TextFieldOverlay` to inherit `{{var}}` highlighting, variable chips, ctrl-arrow word motion, and the shared padding/border.
