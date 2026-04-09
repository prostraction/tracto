package ui

import (
	"encoding/json"
	"io"
	"strings"
	"tracto/internal/utils"

	"github.com/nanorele/gio/widget"
)

type ExtCollection struct {
	Info struct {
		Name string `json:"name"`
	} `json:"info"`
	Item []ExtItem `json:"item"`
}

type ExtItem struct {
	Name    string          `json:"name"`
	Item    []ExtItem       `json:"item"`
	Request json.RawMessage `json:"request"`
}

type ExtRequest struct {
	Method string      `json:"method"`
	URL    interface{} `json:"url"`
	Header interface{} `json:"header"`
	Body   struct {
		Mode string `json:"mode"`
		Raw  string `json:"raw"`
	} `json:"body"`
}

type CollectionNode struct {
	Name       string
	IsFolder   bool
	Request    *ParsedRequest
	Children   []*CollectionNode
	Expanded   bool
	Depth      int
	Click      widget.Clickable
	Parent     *CollectionNode
	Collection *ParsedCollection

	MenuBtn   widget.Clickable
	MenuOpen  bool
	AddReqBtn widget.Clickable
	AddFldBtn widget.Clickable
	EditBtn   widget.Clickable
	DupBtn    widget.Clickable
	DelBtn    widget.Clickable

	IsRenaming bool
	NameEditor widget.Editor
}

type ParsedCollection struct {
	ID   string
	Name string
	Root *CollectionNode
}

type ParsedRequest struct {
	Name    string
	Method  string
	URL     string
	Body    string
	Headers map[string]string
}

type CollectionUI struct {
	Data *ParsedCollection
}

func cloneNode(node *CollectionNode, parent *CollectionNode) *CollectionNode {
	dup := &CollectionNode{
		Name:       node.Name + " Copy",
		IsFolder:   node.IsFolder,
		Depth:      node.Depth,
		Parent:     parent,
		Collection: node.Collection,
	}
	dup.NameEditor.SingleLine = true
	dup.NameEditor.Submit = true

	if node.Request != nil {
		dup.Request = &ParsedRequest{
			Name:   dup.Name,
			Method: node.Request.Method,
			URL:    node.Request.URL,
			Body:   node.Request.Body,
		}
		dup.Request.Headers = make(map[string]string)
		for k, v := range node.Request.Headers {
			dup.Request.Headers[k] = v
		}
	}

	for _, child := range node.Children {
		dup.Children = append(dup.Children, cloneNode(child, dup))
	}
	return dup
}

func assignParents(node *CollectionNode, parent *CollectionNode, col *ParsedCollection) {
	node.Parent = parent
	node.Collection = col
	node.NameEditor.SingleLine = true
	node.NameEditor.Submit = true
	for _, child := range node.Children {
		assignParents(child, node, col)
	}
}

func ParseCollection(r io.Reader, id string) (*ParsedCollection, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var ext ExtCollection
	if err := json.Unmarshal(data, &ext); err != nil {
		return nil, err
	}

	colName := utils.SanitizeText(ext.Info.Name)
	if colName == "" {
		colName = "Imported Collection"
	}

	root := &CollectionNode{
		Name:     colName,
		IsFolder: true,
		Depth:    0,
		Expanded: true,
	}
	root.NameEditor.SingleLine = true
	root.NameEditor.Submit = true

	var parseNode func(items []ExtItem, depth int) []*CollectionNode
	parseNode = func(items []ExtItem, depth int) []*CollectionNode {
		var nodes []*CollectionNode
		for i := range items {
			item := items[i]
			node := &CollectionNode{
				Name:  utils.SanitizeText(item.Name),
				Depth: depth,
			}
			node.NameEditor.SingleLine = true
			node.NameEditor.Submit = true

			if len(item.Item) > 0 {
				node.IsFolder = true
				node.Children = parseNode(item.Item, depth+1)
			}

			if len(item.Request) > 0 && string(item.Request) != "null" {
				var reqObj ExtRequest
				var method string = "GET"
				var url string
				var reqBody string
				headers := make(map[string]string)

				if err := json.Unmarshal(item.Request, &reqObj); err == nil {
					if reqObj.Method != "" {
						method = utils.SanitizeText(reqObj.Method)
					}
					if reqObj.Body.Mode == "raw" {
						reqBody = utils.SanitizeText(reqObj.Body.Raw)
					}

					switch u := reqObj.URL.(type) {
					case string:
						url = utils.SanitizeText(u)
					case map[string]interface{}:
						if raw, ok := u["raw"].(string); ok {
							url = utils.SanitizeText(raw)
						}
					}

					if headerList, ok := reqObj.Header.([]interface{}); ok {
						for _, hObj := range headerList {
							if hMap, ok := hObj.(map[string]interface{}); ok {
								k, _ := hMap["key"].(string)
								v, _ := hMap["value"].(string)
								if k != "" {
									headers[strings.TrimSpace(utils.SanitizeText(k))] = strings.TrimSpace(utils.SanitizeText(v))
								}
							}
						}
					}
				} else {
					var urlStr string
					if err := json.Unmarshal(item.Request, &urlStr); err == nil {
						url = utils.SanitizeText(urlStr)
					}
				}

				node.Request = &ParsedRequest{
					Name:    utils.SanitizeText(item.Name),
					Method:  method,
					URL:     url,
					Body:    reqBody,
					Headers: headers,
				}
			}
			nodes = append(nodes, node)
		}
		return nodes
	}

	root.Children = parseNode(ext.Item, 1)

	col := &ParsedCollection{
		ID:   id,
		Name: colName,
		Root: root,
	}

	assignParents(root, nil, col)

	return col, nil
}
