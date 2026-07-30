package app

import (
	"encoding/xml"
	"testing"
)

func TestUIHierarchyParsesNestedNodes(t *testing.T) {
	data := []byte(`<hierarchy><node class="android.widget.FrameLayout" bounds="[0,0][1080,2400]" enabled="true"><node text="Hello" resource-id="app:id/title" bounds="[0,0][100,80]" clickable="true" /></node></hierarchy>`)
	var hierarchy uiHierarchy
	if err := xml.Unmarshal(data, &hierarchy); err != nil {
		t.Fatal(err)
	}
	if len(hierarchy.Nodes) != 1 || len(hierarchy.Nodes[0].Children) != 1 || hierarchy.Nodes[0].Children[0].Text != "Hello" || !hierarchy.Nodes[0].Children[0].Clickable {
		t.Fatalf("unexpected hierarchy: %#v", hierarchy)
	}
}
