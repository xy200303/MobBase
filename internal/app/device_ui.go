package app

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/xy200303/MobBase/internal/platform/android"
)

type uiHierarchy struct {
	Nodes []uiNode `xml:"node" json:"nodes"`
}

type uiNode struct {
	Class       string   `xml:"class,attr" json:"class,omitempty"`
	Text        string   `xml:"text,attr" json:"text,omitempty"`
	ResourceID  string   `xml:"resource-id,attr" json:"resourceId,omitempty"`
	ContentDesc string   `xml:"content-desc,attr" json:"contentDesc,omitempty"`
	Bounds      string   `xml:"bounds,attr" json:"bounds"`
	Enabled     bool     `xml:"enabled,attr" json:"enabled"`
	Clickable   bool     `xml:"clickable,attr" json:"clickable"`
	Children    []uiNode `xml:"node" json:"children,omitempty"`
}

func (r runtime) deviceUITree(ctx context.Context, args []string) error {
	deviceID := ""
	if len(args) == 2 && args[0] == "--device" {
		deviceID = args[1]
	} else if len(args) != 0 {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Use mob device ui-tree [--device android:<native-id>]."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return err
	}
	devices, err := android.ListDevices(ctx, sdks)
	if err != nil {
		return androidCommandError(err, "Ensure Android platform-tools is installed.")
	}
	device, err := selectAndroidRunDevice(devices, deviceID, config.Device.DefaultID)
	if err != nil {
		return err
	}
	data, err := android.UIHierarchyXML(ctx, sdks, device.NativeID)
	if err != nil {
		return androidCommandError(err, "Verify the selected device remains ready and supports UI Automator.")
	}
	var hierarchy uiHierarchy
	if err := xml.Unmarshal(data, &hierarchy); err != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "Parse Android UI hierarchy: " + err.Error(), Remediation: "Unlock the device and retry mob device ui-tree."}
	}
	if r.json {
		return r.result("mob device ui-tree", map[string]interface{}{"device": device, "nodes": hierarchy.Nodes})
	}
	fmt.Fprintf(r.out, "Android UI hierarchy: %d root node(s) on %s. Use --json for structured nodes.\n", len(hierarchy.Nodes), device.ID)
	return nil
}
