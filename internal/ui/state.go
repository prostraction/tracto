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
)

type HeaderState struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TabState struct {
	Title            string        `json:"title"`
	Method           string        `json:"method"`
	URL              string        `json:"url"`
	Body             string        `json:"body"`
	Headers          []HeaderState `json:"headers"`
	SplitRatio       float32       `json:"split_ratio"`
	HeaderSplitRatio float32       `json:"header_split_ratio,omitempty"`
	ReqWrapEnabled   *bool         `json:"req_wrap_enabled,omitempty"`
	CollectionID     string        `json:"collection_id,omitempty"`
	NodePath         []int         `json:"node_path,omitempty"`
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

// loadStateWithRaw returns both the decoded state and the raw bytes
// read from state.json. Callers that need to detect legacy fields for
// migration (e.g. to trigger a rewrite that drops a removed setting)
// use the raw bytes — json.Unmarshal silently ignores unknown fields,
// so the decoded struct alone can't reveal whether the file had stale
// entries. An empty byte slice means the file didn't exist.
func loadStateWithRaw() (AppState, []byte) {
	var state AppState
	data, err := os.ReadFile(getStateFile())
	if err != nil {
		return state, nil
	}
	json.Unmarshal(data, &state)
	// Backfill defaults for bool/numeric fields added after the on-disk
	// state.json was first written. JSON Unmarshal silently uses Go's
	// zero value for absent keys, but several new settings semantically
	// default to "on" rather than "off" — without this the upgrade flow
	// would silently flip Keep-Alive, JSON formatting, etc. off.
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
	err := os.WriteFile(path, data, 0644)
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
	err := os.WriteFile(path, data, 0644)
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
	return os.WriteFile(path, data, 0644)
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
			if n.Request.Body != "" {
				req.Body.Mode = "raw"
				req.Body.Raw = n.Request.Body
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

func snapshotCollection(col *ParsedCollection) (string, *ExtCollection) {
	if col == nil || col.Root == nil || col.ID == "" {
		return "", nil
	}
	ext := &ExtCollection{}
	ext.Info.Name = col.Name
	ext.Item = buildExtItems(col.Root.Children)
	return col.ID, ext
}

func writeCollectionFile(id string, ext *ExtCollection) error {
	if id == "" || ext == nil {
		return nil
	}
	data, err := json.MarshalIndent(ext, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(getCollectionsDir(), id+".json")
	return os.WriteFile(path, data, 0644)
}

func SaveCollectionToFile(col *ParsedCollection) error {
	id, ext := snapshotCollection(col)
	if ext == nil {
		return nil
	}
	return writeCollectionFile(id, ext)
}
