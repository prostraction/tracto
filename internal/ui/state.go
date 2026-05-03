package ui

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpPath)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

type HeaderState struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TabState struct {
	Title            string          `json:"title"`
	Method           string          `json:"method"`
	URL              string          `json:"url"`
	Body             string          `json:"body"`
	Headers          []HeaderState   `json:"headers"`
	SplitRatio       float32         `json:"split_ratio"`
	HeaderSplitRatio float32         `json:"header_split_ratio,omitempty"`
	ReqWrapEnabled   *bool           `json:"req_wrap_enabled,omitempty"`
	CollectionID     string          `json:"collection_id,omitempty"`
	NodePath         []int           `json:"node_path,omitempty"`
	BodyType         string          `json:"body_type,omitempty"`
	FormParts        []FormPartState `json:"form_parts,omitempty"`
	URLEncoded       []HeaderState   `json:"url_encoded,omitempty"`
	BinaryPath       string          `json:"binary_path,omitempty"`
}

type FormPartState struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	Value    string `json:"value,omitempty"`
	FilePath string `json:"file_path,omitempty"`
}

type AppState struct {
	Tabs               []TabState   `json:"tabs"`
	ActiveIdx          int          `json:"active_idx"`
	ActiveEnvID        string       `json:"active_env_id"`
	SidebarWidthPx     int          `json:"sidebar_width_px"`
	SidebarEnvHeightPx int          `json:"sidebar_env_height_px"`
	Settings           *AppSettings `json:"settings,omitempty"`
}

var configPathOverride string

func newRandomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getConfigPath() string {
	if configPathOverride != "" {
		return configPathOverride
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	appDir := filepath.Join(configDir, "tracto")
	os.MkdirAll(appDir, 0755)
	return appDir
}

func getStateFile() string {
	return filepath.Join(getConfigPath(), "state.json")
}

func getCollectionsDir() string {
	colDir := filepath.Join(getConfigPath(), "collections")
	os.MkdirAll(colDir, 0755)
	return colDir
}

func getEnvironmentsDir() string {
	envDir := filepath.Join(getConfigPath(), "environments")
	os.MkdirAll(envDir, 0755)
	return envDir
}

func loadState() AppState {
	state, _ := loadStateWithRaw()
	return state
}

func loadStateWithRaw() (AppState, []byte) {
	var state AppState
	data, err := os.ReadFile(getStateFile())
	if err != nil {
		return state, nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return state, data
	}
	if err := json.Unmarshal(data, &state); err != nil {
		backup := getStateFile() + ".broken-" + time.Now().Format("20060102-150405")
		os.Rename(getStateFile(), backup)
		return AppState{}, nil
	}

	if state.Settings != nil {
		if !bytes.Contains(data, []byte(`"keep_alive"`)) {
			state.Settings.KeepAlive = true
		}
		if !bytes.Contains(data, []byte(`"auto_format_json"`)) {
			state.Settings.AutoFormatJSON = true
		}
		if !bytes.Contains(data, []byte(`"strip_json_comments"`)) {
			state.Settings.StripJSONComments = true
		}
		if !bytes.Contains(data, []byte(`"default_method"`)) {
			state.Settings.DefaultMethod = "GET"
		}
		if !bytes.Contains(data, []byte(`"default_split_ratio"`)) {
			state.Settings.DefaultSplitRatio = 0.5
		}
		if !bytes.Contains(data, []byte(`"bracket_pair_colorization"`)) {
			state.Settings.BracketPairColorization = true
		}
	}
	return state, data
}

func saveCollectionRaw(data []byte) (string, error) {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	id := hex.EncodeToString(bytes)

	path := filepath.Join(getCollectionsDir(), id+".json")
	err := atomicWriteFile(path, data)
	return id, err
}

func loadSavedCollections() []*ParsedCollection {
	dir := getCollectionsDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var collections []*ParsedCollection
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			path := filepath.Join(dir, f.Name())
			file, err := os.Open(path)
			if err == nil {
				id := strings.TrimSuffix(f.Name(), ".json")
				col, err := ParseCollection(file, id)
				if err == nil && col != nil {
					collections = append(collections, col)
				}
				file.Close()
			}
		}
	}
	return collections
}

func saveEnvironmentRaw(data []byte) (string, error) {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	id := hex.EncodeToString(bytes)

	path := filepath.Join(getEnvironmentsDir(), id+".json")
	err := atomicWriteFile(path, data)
	return id, err
}

func SaveEnvironment(env *ParsedEnvironment) error {
	ext := ExtEnvironment{
		Name: env.Name,
	}
	for _, v := range env.Vars {
		ext.Values = append(ext.Values, struct {
			Key     string `json:"key"`
			Value   string `json:"value"`
			Enabled bool   `json:"enabled"`
		}{
			Key:     v.Key,
			Value:   v.Value,
			Enabled: v.Enabled,
		})
	}
	data, err := json.MarshalIndent(ext, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(getEnvironmentsDir(), env.ID+".json")
	return atomicWriteFile(path, data)
}

func loadSavedEnvironments() []*ParsedEnvironment {
	dir := getEnvironmentsDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var envs []*ParsedEnvironment
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			path := filepath.Join(dir, f.Name())
			file, err := os.Open(path)
			if err == nil {
				id := strings.TrimSuffix(f.Name(), ".json")
				env, err := ParseEnvironment(file, id)
				if err == nil && env != nil {
					envs = append(envs, env)
				}
				file.Close()
			}
		}
	}
	return envs
}

func buildExtItems(nodes []*CollectionNode) []ExtItem {
	var items []ExtItem
	for _, n := range nodes {
		item := ExtItem{Name: n.Name}
		if n.IsFolder {
			item.Item = buildExtItems(n.Children)
		} else if n.Request != nil {
			req := ExtRequest{Method: n.Request.Method, URL: n.Request.URL}
			req.Body.Mode = n.Request.BodyType.PostmanMode()
			switch n.Request.BodyType {
			case BodyRaw:
				if n.Request.Body != "" {
					req.Body.Raw = n.Request.Body
				}
			case BodyURLEncoded:
				for _, kv := range n.Request.URLEncoded {
					if kv.Key == "" {
						continue
					}
					req.Body.URLEncoded = append(req.Body.URLEncoded, ExtKVPart{
						Key: kv.Key, Value: kv.Value,
					})
				}
			case BodyFormData:
				for _, fp := range n.Request.FormParts {
					if fp.Key == "" {
						continue
					}
					part := ExtFormPart{Key: fp.Key, Type: "text", Value: fp.Value}
					if fp.Kind == FormPartFile {
						part.Type = "file"
						part.Value = ""
						if fp.FilePath != "" {
							part.Src = fp.FilePath
						}
					}
					req.Body.FormData = append(req.Body.FormData, part)
				}
			case BodyBinary:
				if n.Request.BinaryPath != "" {
					req.Body.File = &ExtBodyFile{Src: n.Request.BinaryPath}
				}
			}
			if len(n.Request.Headers) > 0 {
				keys := make([]string, 0, len(n.Request.Headers))
				for k := range n.Request.Headers {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				headers := make([]map[string]interface{}, 0, len(keys))
				for _, k := range keys {
					headers = append(headers, map[string]interface{}{"key": k, "value": n.Request.Headers[k]})
				}
				req.Header = headers
			}
			reqBytes, _ := json.Marshal(req)
			item.Request = reqBytes
		}
		items = append(items, item)
	}
	return items
}

func marshalCollection(col *ParsedCollection) []byte {
	info := map[string]any{}
	for k, v := range col.InfoExtras {
		info[k] = v
	}
	info["name"] = col.Name

	items := make([]any, 0, len(col.Root.Children)+len(col.Root.skippedItems))
	for _, child := range col.Root.Children {
		items = append(items, marshalNode(child))
	}

	for _, raw := range col.Root.skippedItems {
		items = append(items, raw)
	}

	out := map[string]any{}
	for k, v := range col.TopExtras {
		out[k] = v
	}
	out["info"] = info
	out["item"] = items

	data, _ := json.MarshalIndent(out, "", "  ")
	return data
}

func marshalNode(node *CollectionNode) map[string]any {
	out := map[string]any{}
	for k, v := range node.Extras {
		out[k] = v
	}
	out["name"] = node.Name
	if node.IsFolder {
		children := make([]any, 0, len(node.Children)+len(node.skippedItems))
		for _, c := range node.Children {
			children = append(children, marshalNode(c))
		}

		for _, raw := range node.skippedItems {
			children = append(children, raw)
		}
		out["item"] = children
	} else if node.Request != nil {
		out["request"] = marshalRequest(node.Request)
	}
	return out
}

func marshalRequest(req *ParsedRequest) map[string]any {
	out := map[string]any{}
	for k, v := range req.Extras {
		out[k] = v
	}
	out["method"] = req.Method

	if len(req.RawURL) > 0 {
		var urlObj map[string]any
		if err := json.Unmarshal(req.RawURL, &urlObj); err == nil {
			urlObj["raw"] = req.URL
			out["url"] = urlObj
		} else {
			out["url"] = req.URL
		}
	} else {
		out["url"] = req.URL
	}

	out["header"] = marshalRequestHeaders(req)
	out["body"] = marshalRequestBody(req)
	return out
}

func marshalRequestHeaders(req *ParsedRequest) []any {
	if len(req.Headers) == 0 {
		return []any{}
	}
	keys := make([]string, 0, len(req.Headers))
	for k := range req.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{"key": k, "value": req.Headers[k]})
	}
	return out
}

func marshalRequestBody(req *ParsedRequest) map[string]any {
	out := map[string]any{}
	for k, v := range req.BodyExtras {
		out[k] = v
	}
	out["mode"] = req.BodyType.PostmanMode()
	switch req.BodyType {
	case BodyRaw:
		if req.Body != "" {
			out["raw"] = req.Body
		}
	case BodyURLEncoded:
		arr := make([]any, 0, len(req.URLEncoded))
		for _, kv := range req.URLEncoded {
			if kv.Key == "" {
				continue
			}
			arr = append(arr, map[string]any{"key": kv.Key, "value": kv.Value})
		}
		out["urlencoded"] = arr
	case BodyFormData:
		arr := make([]any, 0, len(req.FormParts))
		for _, fp := range req.FormParts {
			if fp.Key == "" {
				continue
			}
			row := map[string]any{"key": fp.Key, "type": "text", "value": fp.Value}
			if fp.Kind == FormPartFile {
				row["type"] = "file"
				delete(row, "value")
				if fp.FilePath != "" {
					row["src"] = fp.FilePath
				}
			}
			arr = append(arr, row)
		}
		out["formdata"] = arr
	case BodyBinary:
		if req.BinaryPath != "" {
			out["file"] = map[string]any{"src": req.BinaryPath}
		}
	}
	return out
}

func snapshotCollection(col *ParsedCollection) (string, []byte) {
	if col == nil || col.Root == nil || col.ID == "" {
		return "", nil
	}
	return col.ID, marshalCollection(col)
}

func writeCollectionFile(id string, data []byte) error {
	if id == "" || len(data) == 0 {
		return nil
	}
	path := filepath.Join(getCollectionsDir(), id+".json")
	return atomicWriteFile(path, data)
}

func SaveCollectionToFile(col *ParsedCollection) error {
	id, data := snapshotCollection(col)
	if len(data) == 0 {
		return nil
	}
	return writeCollectionFile(id, data)
}
