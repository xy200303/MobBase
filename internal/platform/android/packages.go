package android

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/xy200303/MobBase/internal/system"
)

// ListAvailableSystemImages asks the installed official sdkmanager for the
// current host's image packages. Google does not publish every image in
// repository2-1.xml, so sdkmanager is authoritative after SDK bootstrap.
func ListAvailableSystemImages(ctx context.Context, root string) ([]CatalogItem, error) {
	manager, found := SDKManager(root)
	if !found {
		return nil, fmt.Errorf("Android SDK command-line tools were not found in %s", root)
	}
	program, arguments := system.BatchCommand(manager, "--sdk_root="+root, "--list")
	result, err := system.Run(ctx, program, arguments, nil, "", "")
	if err != nil {
		return nil, fmt.Errorf("list Android SDK packages: %w: %s", err, strings.TrimSpace(result.Output))
	}
	return ParseAvailableSystemImages(result.Output), nil
}

func ParseAvailableSystemImages(output string) []CatalogItem {
	items := make([]CatalogItem, 0)
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		columns := strings.Split(line, "|")
		if len(columns) < 2 {
			continue
		}
		packageID := strings.TrimSpace(columns[0])
		if !strings.HasPrefix(packageID, "system-images;") || seen[packageID] {
			continue
		}
		seen[packageID] = true
		item := CatalogItem{PackageID: packageID, Version: strings.TrimSpace(columns[1])}
		if len(columns) > 2 {
			item.DisplayName = strings.TrimSpace(columns[2])
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].PackageID < items[j].PackageID })
	return items
}

func HasAvailableSystemImage(ctx context.Context, root, packageID string) (bool, error) {
	items, err := ListAvailableSystemImages(ctx, root)
	if err != nil {
		return false, err
	}
	return ContainsPackage(items, packageID), nil
}
