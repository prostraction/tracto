package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"tracto/internal/utils"
)

const previewBatchSize = 3 * 1024 * 1024
const jsonPreviewBatchSize = 2 * 1024 * 1024
const jsonPrettyMaxSize = 1024 * 1024

var previewBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, previewBatchSize)
		return &b
	},
}

func getPreviewBuf(size int64) ([]byte, func()) {
	if size <= previewBatchSize {
		bp := previewBufPool.Get().(*[]byte)
		buf := (*bp)[:size]
		return buf, func() { previewBufPool.Put(bp) }
	}
	buf := make([]byte, size)
	return buf, func() {}
}

var indentTable [64]string

func init() {
	for i := range indentTable {
		indentTable[i] = "\n" + strings.Repeat("  ", i)
	}
}

type JSONFormatterState struct {
	Indent     int
	InString   bool
	NeedIndent bool
	EscapeNext bool
}

func indentWrite(out *strings.Builder, indent int) {
	if indent >= len(indentTable) {
		indent = len(indentTable) - 1
	}
	out.WriteString(indentTable[indent])
}

func formatJSON(data []byte, state *JSONFormatterState) string {
	var out strings.Builder
	out.Grow(len(data) * 3)

	i := 0
	for i < len(data) {
		if state.InString {
			start := i
			if !state.EscapeNext {
				idx := bytes.IndexAny(data[i:], "\"\\")
				if idx == -1 {
					out.Write(data[start:])
					break
				}
				i += idx
			}
			
			if i > start {
				out.Write(data[start:i])
			}
			
			b := data[i]
			i++
			
			if state.EscapeNext {
				out.WriteByte(b)
				state.EscapeNext = false
			} else if b == '\\' {
				out.WriteByte('\\')
				state.EscapeNext = true
			} else {
				out.WriteByte('"')
				state.InString = false
			}
			continue
		}

		b := data[i]
		i++

		switch b {
		case '"':
			if state.NeedIndent {
				indentWrite(&out, state.Indent)
				state.NeedIndent = false
			}
			out.WriteByte('"')
			state.InString = true
		case '{', '[':
			if state.NeedIndent {
				indentWrite(&out, state.Indent)
				state.NeedIndent = false
			}
			out.WriteByte(b)
			
			// Lookahead to format empty arrays/objects on a single line
			j := i
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && ((b == '{' && data[j] == '}') || (b == '[' && data[j] == ']')) {
				out.WriteByte(data[j])
				i = j + 1
				continue
			}

			state.Indent++
			state.NeedIndent = true
		case '}', ']':
			state.Indent--
			if state.Indent < 0 {
				state.Indent = 0
			}
			indentWrite(&out, state.Indent)
			out.WriteByte(b)
		case ',':
			out.WriteByte(',')
			state.NeedIndent = true
		case ':':
			out.WriteByte(':')
			out.WriteByte(' ')
		case ' ', '\t', '\n', '\r':
		default:
			if state.NeedIndent {
				indentWrite(&out, state.Indent)
				state.NeedIndent = false
			}
			start := i - 1
			idx := bytes.IndexAny(data[i:], ",}]: \t\n\r")
			if idx == -1 {
				out.Write(data[start:])
				i = len(data)
			} else {
				out.Write(data[start : i+idx])
				i += idx
			}
		}
	}
	return out.String()
}

func looksLikeJSON(data []byte) bool {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{', '[':
			return true
		default:
			return false
		}
	}
	return false
}

func loadPreviewFromFile(path string, totalSize int64, state *JSONFormatterState) (string, int64, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, false
	}
	defer f.Close()

	var probe [64]byte
	pn, _ := f.Read(probe[:])
	isJSON := looksLikeJSON(probe[:pn])
	if isJSON && totalSize >= jsonPrettyMaxSize {
		isJSON = false
	}

	batchSize := int64(previewBatchSize)
	if isJSON {
		batchSize = int64(jsonPreviewBatchSize)
	}
	readSize := totalSize
	if readSize > batchSize {
		readSize = batchSize
	}

	f.Seek(0, io.SeekStart)
	data, release := getPreviewBuf(readSize)
	n, _ := io.ReadFull(f, data)
	data = data[:n]

	var result string
	if isJSON {
		if readSize == totalSize {
			var buf bytes.Buffer
			if err := json.Indent(&buf, data, "", "  "); err == nil {
				result = buf.String()
			} else {
				result = formatJSON(data, state)
			}
		} else {
			result = formatJSON(data, state)
		}
	} else {
		result = utils.SanitizeBytes(data)
	}
	release()
	return result, int64(n), isJSON
}

func (t *RequestTab) loadMorePreview() {
	if t.respFile == "" || t.previewLoaded >= t.respSize {
		return
	}

	filePath := t.respFile
	offset := t.previewLoaded
	batchLimit := int64(previewBatchSize)
	if t.respIsJSON {
		batchLimit = int64(jsonPreviewBatchSize)
	}
	readSize := t.respSize - t.previewLoaded
	if readSize > batchLimit {
		readSize = batchLimit
	}
	t.previewLoaded += readSize
	win := t.window
	isJSON := t.respIsJSON

	go func() {
		f, err := os.Open(filePath)
		if err != nil {
			return
		}
		defer f.Close()
		f.Seek(offset, io.SeekStart)

		data, release := getPreviewBuf(readSize)
		n, _ := io.ReadFull(f, data)
		data = data[:n]

		var extra string
		if isJSON {
			extra = formatJSON(data, t.jsonFmtState)
		} else {
			extra = utils.SanitizeBytes(data)
		}
		release()
		t.streamToEditor(extra, win)
	}()
}

func openFile(path string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("cmd", "/c", "start", "", path).Start()
	case "darwin":
		exec.Command("open", path).Start()
	default:
		exec.Command("xdg-open", path).Start()
	}
}

func openFileInExplorer(path string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("explorer", "/select,", filepath.ToSlash(path)).Start()
	case "darwin":
		exec.Command("open", "-R", path).Start()
	default:
		dir := filepath.Dir(path)
		exec.Command("xdg-open", dir).Start()
	}
}
