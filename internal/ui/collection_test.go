package ui

import (
	"strings"
	"testing"
)

func TestNodePathFromAndAtPath(t *testing.T) {
	root := &CollectionNode{Name: "root", IsFolder: true}
	child1 := &CollectionNode{Name: "child1", Parent: root}
	child2 := &CollectionNode{Name: "child2", Parent: root}
	subchild := &CollectionNode{Name: "subchild", Parent: child2}
	root.Children = []*CollectionNode{child1, child2}
	child2.Children = []*CollectionNode{subchild}

	path := nodePathFrom(root, subchild)
	if len(path) != 2 || path[0] != 1 || path[1] != 0 {
		t.Errorf("expected [1, 0], got %v", path)
	}

	pathRoot := nodePathFrom(root, root)
	if pathRoot != nil {
		t.Errorf("expected nil path for root, got %v", pathRoot)
	}

	pathNilTarget := nodePathFrom(root, nil)
	if pathNilTarget != nil {
		t.Errorf("expected nil path for nil target, got %v", pathNilTarget)
	}

	pathNilRoot := nodePathFrom(nil, subchild)
	if pathNilRoot != nil {
		t.Errorf("expected nil path for nil root, got %v", pathNilRoot)
	}

	unrelatedNode := &CollectionNode{Name: "unrelated"}
	pathUnrelated := nodePathFrom(root, unrelatedNode)
	if pathUnrelated != nil {
		t.Errorf("expected nil path for unrelated target, got %v", pathUnrelated)
	}

	detachedParent := &CollectionNode{Name: "detached"}
	detachedChild := &CollectionNode{Name: "child", Parent: detachedParent}
	pathDetached := nodePathFrom(root, detachedChild)
	if pathDetached != nil {
		t.Errorf("expected nil path for detached child, got %v", pathDetached)
	}

	found := nodeAtPath(root, []int{1, 0})
	if found != subchild {
		t.Errorf("expected subchild, got %v", found)
	}

	notFound := nodeAtPath(root, []int{2, 0})
	if notFound != nil {
		t.Errorf("expected nil, got %v", notFound)
	}

	notFound2 := nodeAtPath(root, []int{1, 1})
	if notFound2 != nil {
		t.Errorf("expected nil, got %v", notFound2)
	}

	notFound3 := nodeAtPath(root, []int{-1})
	if notFound3 != nil {
		t.Errorf("expected nil, got %v", notFound3)
	}
}

func TestCloneNode(t *testing.T) {
	col := &ParsedCollection{ID: "col1"}
	root := &CollectionNode{
		Name:       "req",
		IsFolder:   false,
		Depth:      1,
		Collection: col,
		Request: &ParsedRequest{
			Name:   "req",
			Method: "POST",
			URL:    "http://example.com",
			Body:   "{}",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		},
	}

	clone := cloneNode(root, nil)

	if clone.Name != "req Copy" {
		t.Errorf("expected req Copy, got %s", clone.Name)
	}
	if clone.Parent != nil {
		t.Errorf("expected nil parent")
	}
	if clone.Collection != col {
		t.Errorf("expected same collection")
	}
	if clone.Request == nil {
		t.Fatalf("expected request")
	}
	if clone.Request.Name != "req Copy" {
		t.Errorf("expected request name req Copy, got %s", clone.Request.Name)
	}
	if clone.Request.Method != "POST" {
		t.Errorf("expected POST, got %s", clone.Request.Method)
	}
	if clone.Request.Headers["Content-Type"] != "application/json" {
		t.Errorf("expected header application/json")
	}

	root.Request.Headers["Content-Type"] = "text/plain"
	if clone.Request.Headers["Content-Type"] != "application/json" {
		t.Errorf("expected clone to retain original header, got %s", clone.Request.Headers["Content-Type"])
	}

	folder := &CollectionNode{
		Name:     "folder",
		IsFolder: true,
		Children: []*CollectionNode{root},
	}
	cloneFolder := cloneNode(folder, nil)
	if len(cloneFolder.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(cloneFolder.Children))
	}
	if cloneFolder.Children[0].Parent != cloneFolder {
		t.Errorf("expected child parent to be cloned folder")
	}
}

func TestParseCollection(t *testing.T) {
	jsonStr := `
	{
		"info": {
			"name": "Test Collection"
		},
		"item": [
			{
				"name": "Folder",
				"item": [
					{
						"name": "Request 1",
						"request": {
							"method": "GET",
							"url": "http://example.com",
							"header": [
								{
									"key": "Accept",
									"value": "application/json"
								}
							],
							"body": {
								"mode": "raw",
								"raw": "test"
							}
						}
					},
					{
						"name": "Request 2",
						"request": {
							"method": "POST",
							"url": {
								"raw": "http://example.com/api"
							}
						}
					}
				]
			},
			{
				"name": "Request 3 string URL",
				"request": "http://example.com/string"
			}
		]
	}`

	col, err := ParseCollection(strings.NewReader(jsonStr), "id1")
	if err != nil {
		t.Fatalf("ParseCollection error: %v", err)
	}

	if col.ID != "id1" {
		t.Errorf("expected id1, got %s", col.ID)
	}
	if col.Name != "Test Collection" {
		t.Errorf("expected Test Collection, got %s", col.Name)
	}
	if col.Root == nil {
		t.Fatalf("expected root node")
	}
	if len(col.Root.Children) != 2 {
		t.Fatalf("expected 2 children at root, got %d", len(col.Root.Children))
	}

	folder := col.Root.Children[0]
	if !folder.IsFolder {
		t.Errorf("expected folder to be IsFolder")
	}
	if len(folder.Children) != 2 {
		t.Fatalf("expected 2 requests in folder")
	}

	req1 := folder.Children[0]
	if req1.Request == nil {
		t.Fatalf("expected request")
	}
	if req1.Request.Method != "GET" {
		t.Errorf("expected GET, got %s", req1.Request.Method)
	}
	if req1.Request.URL != "http://example.com" {
		t.Errorf("expected url, got %s", req1.Request.URL)
	}
	if req1.Request.Body != "test" {
		t.Errorf("expected test, got %s", req1.Request.Body)
	}
	if req1.Request.Headers["Accept"] != "application/json" {
		t.Errorf("expected header Accept: application/json")
	}

	req2 := folder.Children[1]
	if req2.Request.URL != "http://example.com/api" {
		t.Errorf("expected url, got %s", req2.Request.URL)
	}

	req3 := col.Root.Children[1]
	if req3.Request.URL != "http://example.com/string" {
		t.Errorf("expected string url, got %s", req3.Request.URL)
	}

	_, err = ParseCollection(strings.NewReader("invalid"), "id2")
	if err == nil {
		t.Errorf("expected error for invalid json")
	}

	jsonWithItems := `{"info": {"name": ""}, "item": [{"name": "Req"}]}`
	colEmpty, _ := ParseCollection(strings.NewReader(jsonWithItems), "id3")
	if colEmpty.Name != "Imported Collection" {
		t.Errorf("expected Imported Collection, got %s", colEmpty.Name)
	}

	jsonReallyEmpty := `{"info": {"name": ""}, "item": []}`
	_, err = ParseCollection(strings.NewReader(jsonReallyEmpty), "id4")
	if err == nil {
		t.Errorf("expected error for really empty collection")
	}
}

func TestAssignParents(t *testing.T) {
	col := &ParsedCollection{}
	root := &CollectionNode{Name: "root"}
	child := &CollectionNode{Name: "child"}
	subchild := &CollectionNode{Name: "subchild"}

	root.Children = append(root.Children, child)
	child.Children = append(child.Children, subchild)

	assignParents(root, nil, col)

	if child.Parent != root {
		t.Errorf("expected child parent to be root")
	}
	if child.Collection != col {
		t.Errorf("expected child collection to be col")
	}
	if subchild.Parent != child {
		t.Errorf("expected subchild parent to be child")
	}
	if !child.NameEditor.SingleLine {
		t.Errorf("expected NameEditor.SingleLine true")
	}
}
