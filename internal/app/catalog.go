package app

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/android"
	javaplatform "github.com/xy200303/MobBase/internal/platform/java"
	"github.com/xy200303/MobBase/internal/state"
)

type sdkAvailableOptions struct {
	API     int
	Refresh bool
}

type catalogOptions struct {
	Platform, Component string
	Refresh             bool
}

func (r runtime) catalog(ctx context.Context, args []string) error {
	options, err := parseCatalog(args)
	if err != nil {
		return err
	}
	if options.Platform != "" && options.Platform != "android" {
		return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: "Only Android catalog entries are available in this Mob release."}
	}
	catalog, err := android.LoadCatalog(ctx, android.CatalogCachePath(r.home), options.Refresh)
	if err != nil {
		return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Check access to the Android official repository, or retry with a valid cached catalog."}
	}
	javaCatalog, err := javaplatform.LoadCatalog(ctx, javaplatform.CachePath(r.home), options.Refresh)
	if err != nil {
		return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Check access to the Eclipse Temurin official API, or retry with a valid cached catalog."}
	}
	data := map[string]interface{}{"source": catalog.Source, "refreshedAt": catalog.RefreshedAt, "cached": catalog.Cached, "androidSdk": catalog.SDKItems(0), "androidNdk": catalog.NDKItems(), "androidSystemImages": catalog.SystemImageItems(0), "java": javaCatalog.Releases, "javaSource": javaCatalog.Source, "javaRefreshedAt": javaCatalog.RefreshedAt, "javaCached": javaCatalog.Cached}
	if options.Component != "" {
		for _, key := range []string{"androidSdk", "androidNdk", "androidSystemImages", "java"} {
			data[key] = nil
		}
		switch options.Component {
		case "sdk":
			data["androidSdk"] = catalog.SDKItems(0)
		case "ndk":
			data["androidNdk"] = catalog.NDKItems()
		case "system-image":
			data["androidSystemImages"] = catalog.SystemImageItems(0)
		case "java":
			data["java"] = javaCatalog.Releases
		}
	}
	if r.json {
		return r.result("mob catalog", data)
	}
	for _, group := range []struct {
		name, component string
		items           []android.CatalogItem
	}{{"Android SDK", "sdk", catalog.SDKItems(0)}, {"Android NDK", "ndk", catalog.NDKItems()}, {"Android system images", "system-image", catalog.SystemImageItems(0)}} {
		if options.Component != "" && options.Component != group.component {
			continue
		}
		fmt.Fprintln(r.out, group.name+":")
		for _, item := range group.items {
			fmt.Fprintf(r.out, "  %s\t%s\t%d\n", item.PackageID, item.Version, item.Size)
		}
	}
	if options.Component == "" || options.Component == "java" {
		fmt.Fprintln(r.out, "JDK:")
		for _, release := range javaCatalog.Releases {
			fmt.Fprintf(r.out, "  %d\t%s\t%s\n", release.Major, release.Version, release.SHA256)
		}
	}
	return nil
}

func parseCatalog(args []string) (catalogOptions, error) {
	var options catalogOptions
	for len(args) > 0 {
		if args[0] == "--refresh" {
			options.Refresh = true
			args = args[1:]
			continue
		}
		if (args[0] == "--platform" || args[0] == "--component") && len(args) >= 2 && strings.TrimSpace(args[1]) != "" {
			if args[0] == "--platform" {
				options.Platform = args[1]
			} else {
				options.Component = args[1]
			}
			args = args[2:]
			continue
		}
		return catalogOptions{}, invalidCommand("mob catalog " + strings.Join(args, " "))
	}
	if options.Component != "" && options.Component != "sdk" && options.Component != "ndk" && options.Component != "system-image" && options.Component != "java" {
		return catalogOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--component must be sdk, ndk, system-image, or java."}
	}
	return options, nil
}

func (r runtime) sdkAvailable(ctx context.Context, args []string) error {
	options, err := parseSDKAvailable(args)
	if err != nil {
		return err
	}
	catalog, err := android.LoadCatalog(ctx, android.CatalogCachePath(r.home), options.Refresh)
	if err != nil {
		return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Check network access to the Android official repository, or retry later with a valid cached catalog."}
	}
	items := catalog.SDKItems(options.API)
	data := map[string]interface{}{"source": catalog.Source, "refreshedAt": catalog.RefreshedAt, "cached": catalog.Cached, "items": items}
	if r.json {
		return r.result("mob android sdk available", data)
	}
	if len(items) == 0 {
		fmt.Fprintln(r.out, "No matching Android SDK packages are available.")
		return nil
	}
	for _, item := range items {
		fmt.Fprintf(r.out, "%s\t%s\t%d\t%s\n", item.PackageID, item.Version, item.Size, item.ChecksumAlgorithm)
	}
	return nil
}

func parseSDKAvailable(args []string) (sdkAvailableOptions, error) {
	options := sdkAvailableOptions{}
	for len(args) > 0 {
		switch args[0] {
		case "--api":
			if len(args) < 2 {
				return sdkAvailableOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--api requires an Android API level."}
			}
			api, err := strconv.Atoi(args[1])
			if err != nil || api < 1 {
				return sdkAvailableOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--api must be a positive integer."}
			}
			options.API = api
			args = args[2:]
		case "--refresh":
			options.Refresh = true
			args = args[1:]
		default:
			return sdkAvailableOptions{}, invalidCommand("mob android sdk available " + strings.Join(args, " "))
		}
	}
	return options, nil
}

func (r runtime) emulatorImageAvailable(ctx context.Context, args []string) error {
	options, err := parseSDKAvailable(args)
	if err != nil {
		return err
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	if sdks, discoverErr := android.Discover(config); discoverErr == nil {
		for _, sdk := range preferredSDKs(sdks) {
			if _, found := android.SDKManager(sdk.Path); !found {
				continue
			}
			items, listErr := android.ListAvailableSystemImages(ctx, sdk.Path)
			if listErr != nil {
				return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: listErr.Error(), Remediation: "Check Android repository access and retry."}
			}
			items = filterSystemImages(items, options.API)
			data := map[string]interface{}{"source": "sdkmanager", "cached": false, "sdk": sdk.Name, "items": items}
			if r.json {
				return r.result("mob android emulator image available", data)
			}
			writeSystemImageTable(r.out, items, "sdkmanager", sdk.Name)
			return nil
		}
	}
	catalog, err := android.LoadCatalog(ctx, android.CatalogCachePath(r.home), options.Refresh)
	if err != nil {
		return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Check access to the Android official repository or retry with a cached catalog."}
	}
	items := catalog.SystemImageItems(options.API)
	data := map[string]interface{}{"source": catalog.Source, "refreshedAt": catalog.RefreshedAt, "cached": catalog.Cached, "items": items}
	if r.json {
		return r.result("mob android emulator image available", data)
	}
	writeSystemImageTable(r.out, items, "Android catalog", "")
	return nil
}

func writeSystemImageTable(output io.Writer, items []android.CatalogItem, source, sdk string) {
	title := "Available Android system images"
	if source != "" {
		title += " (source: " + source
		if sdk != "" {
			title += "; SDK: " + sdk
		}
		title += ")"
	}
	fmt.Fprintln(output, title)
	if len(items) == 0 {
		fmt.Fprintln(output, "  No matching images are available.")
		return
	}
	fmt.Fprintf(output, "  %-5s %-34s %-12s %s\n", "API", "IMAGE", "ABI", "REV")
	for _, item := range items {
		api, image, abi, parsed := systemImageFields(item.PackageID)
		if !parsed {
			fmt.Fprintf(output, "  %-5s %-34s %-12s %s\n", "-", item.PackageID, "-", item.Version)
			continue
		}
		fmt.Fprintf(output, "  %-5s %-34s %-12s %s\n", api, image, abi, item.Version)
	}
	fmt.Fprintln(output, "\nInstall one image:")
	fmt.Fprintln(output, "  mob android emulator image install \"system-images;android-<API>;<IMAGE>;<ABI>\" --sdk <sdk-name> --accept-licenses")
	fmt.Fprintln(output, "  Use --json when a script needs full package IDs and metadata.")
}

func systemImageFields(packageID string) (api, image, abi string, parsed bool) {
	parts := strings.Split(packageID, ";")
	if len(parts) != 4 || parts[0] != "system-images" || !strings.HasPrefix(parts[1], "android-") {
		return "", "", "", false
	}
	return strings.TrimPrefix(parts[1], "android-"), parts[2], parts[3], true
}

func preferredSDKs(sdks []android.SDK) []android.SDK {
	ordered := make([]android.SDK, 0, len(sdks))
	for _, sdk := range sdks {
		if sdk.Current {
			ordered = append(ordered, sdk)
		}
	}
	for _, sdk := range sdks {
		if !sdk.Current && sdk.Ownership == state.OwnershipManaged {
			ordered = append(ordered, sdk)
		}
	}
	for _, sdk := range sdks {
		if !sdk.Current && sdk.Ownership != state.OwnershipManaged {
			ordered = append(ordered, sdk)
		}
	}
	return ordered
}

func filterSystemImages(items []android.CatalogItem, api int) []android.CatalogItem {
	if api == 0 {
		return items
	}
	prefix := fmt.Sprintf("system-images;android-%d;", api)
	filtered := make([]android.CatalogItem, 0)
	for _, item := range items {
		if strings.HasPrefix(item.PackageID, prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (r runtime) emulatorImageInstall(ctx context.Context, args []string) error {
	if len(args) < 3 || !strings.HasPrefix(args[0], "system-images;") {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "A system image package ID and --sdk <name> are required.", Remediation: "Use mob android emulator image available, then mob android emulator image install <package-id> --sdk managed --accept-licenses."}
	}
	packageID := args[0]
	sdkName := ""
	forwarded := []string{}
	for rest := args[1:]; len(rest) > 0; {
		if rest[0] == "--sdk" {
			if len(rest) < 2 || strings.TrimSpace(rest[1]) == "" {
				return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--sdk requires an Android SDK name."}
			}
			sdkName = rest[1]
			rest = rest[2:]
			continue
		}
		forwarded = append(forwarded, rest[0])
		rest = rest[1:]
	}
	if sdkName == "" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--sdk is required for system image installation."}
	}
	return r.sdkInstallAs(ctx, append([]string{sdkName, "--package", packageID}, forwarded...), "mob android emulator image install")
}

func (r runtime) ndkAvailable(ctx context.Context, args []string) error {
	refresh, err := parseRefresh(args, "mob android ndk available")
	if err != nil {
		return err
	}
	catalog, err := android.LoadCatalog(ctx, android.CatalogCachePath(r.home), refresh)
	if err != nil {
		return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Check network access to the Android official repository, or retry later with a valid cached catalog."}
	}
	items := catalog.NDKItems()
	data := map[string]interface{}{"source": catalog.Source, "refreshedAt": catalog.RefreshedAt, "cached": catalog.Cached, "items": items}
	if r.json {
		return r.result("mob android ndk available", data)
	}
	for _, item := range items {
		fmt.Fprintf(r.out, "%s\t%s\t%d\t%s\n", item.PackageID, item.Version, item.Size, item.ChecksumAlgorithm)
	}
	return nil
}

func parseRefresh(args []string, command string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--refresh" {
		return true, nil
	}
	return false, invalidCommand(command + " " + strings.Join(args, " "))
}
