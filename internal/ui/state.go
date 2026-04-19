package ui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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
	Tabs               []TabState `json:"tabs"`
	ActiveIdx          int        `json:"active_idx"`
	ActiveEnvID        string     `json:"active_env_id"`
	SidebarWidthPx     int        `json:"sidebar_width_px"`
	SidebarEnvHeightPx int        `json:"sidebar_env_height_px"`
}

var configPathOverride string

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
	var state AppState
	data, err := os.ReadFile(getStateFile())
	if err == nil {
		json.Unmarshal(data, &state)
	}
	return state
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
			var headers []map[string]interface{}
			for k, v := range n.Request.Headers {
				headers = append(headers, map[string]interface{}{"key": k, "value": v})
			}
			if len(headers) > 0 {
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
