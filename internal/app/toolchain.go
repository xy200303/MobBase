package app

import (
	"context"
	"fmt"
	goruntime "runtime"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
	"github.com/xy200303/MobBase/internal/state"
)

// prepareAndroidSDK selects a complete existing SDK or prepares only the
// missing components in android:managed. Discovered and imported SDKs remain
// read-only, including during automatic preparation.
func (r runtime) prepareAndroidSDK(ctx context.Context, root, command string, requireDevice, noInstall, acceptLicenses bool) (android.SDK, project.AndroidRequirements, error) {
	requirements, err := project.AndroidRequirementsFor(root)
	if err != nil {
		return android.SDK{}, project.AndroidRequirements{}, err
	}
	config, err := r.store.Load()
	if err != nil {
		return android.SDK{}, project.AndroidRequirements{}, err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return android.SDK{}, project.AndroidRequirements{}, err
	}
	if sdk, found := matchingAndroidSDK(sdks, requirements, requireDevice); found {
		return sdk, requirements, nil
	}
	if noInstall {
		return android.SDK{}, requirements, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "No installed Android SDK satisfies this project's requirements.", Remediation: "Remove --no-install, or install the required Android packages into android:managed."}
	}
	target, err := r.installTarget("managed", sdks)
	if err != nil {
		return android.SDK{}, requirements, err
	}
	packages := requiredAndroidPackages(requirements, requireDevice)
	if len(packages) == 0 {
		return android.SDK{}, requirements, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "The Android project does not declare a numeric compileSdk version.", Remediation: "Declare compileSdk in the Gradle project, or install and select a complete Android SDK manually."}
	}
	if !acceptLicenses {
		return android.SDK{}, requirements, &codedError{Code: "MOB_LICENSE_REQUIRED", Message: "Android SDK components are missing and require license acceptance before Mob can install them.", Remediation: "Review the Android SDK license, then repeat with --accept-licenses; use --no-install to disable automatic setup."}
	}
	catalog, err := r.validateCatalogPackages(ctx, packages)
	if err != nil {
		return android.SDK{}, requirements, err
	}
	if err := r.emit("progress", command, true, map[string]interface{}{"phase": "prepare-toolchain", "sdk": target.Name, "packages": packages}, nil); err != nil {
		return android.SDK{}, requirements, err
	}
	if err := r.bootstrapManagedSDK(ctx, target, catalog, command); err != nil {
		return android.SDK{}, requirements, err
	}
	if _, err := android.InstallPackages(ctx, android.InstallRequest{Root: target.Path, Packages: packages, AcceptLicenses: true, Environment: androidProxyEnvironment(config), Output: r.sdkManagerOutput()}); err != nil {
		return android.SDK{}, requirements, &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Check Android repository access and the accepted SDK licenses, then retry."}
	}
	r.registerManagedSDK(&config, target)
	if err := r.store.Save(config); err != nil {
		return android.SDK{}, requirements, err
	}
	sdks, err = android.Discover(config)
	if err != nil {
		return android.SDK{}, requirements, err
	}
	sdk, found := matchingAndroidSDK(sdks, requirements, requireDevice)
	if !found {
		return android.SDK{}, requirements, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Mob installed Android components but they could not be discovered.", Remediation: "Run mob android sdk inspect managed and verify the installation."}
	}
	return sdk, requirements, nil
}

// prepareAndroidEmulator adds the official Emulator and one matching system
// image when run needs to create a default AVD.
func (r runtime) prepareAndroidEmulator(ctx context.Context, sdk android.SDK, requirements project.AndroidRequirements, command string, noInstall, acceptLicenses bool) (android.SDK, error) {
	if sdk.Components.Emulator && hasSystemImageForAPI(sdk, requirements.CompileSDK) {
		return sdk, nil
	}
	if noInstall {
		return android.SDK{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "No ready device or emulator system image is available.", Remediation: "Remove --no-install, connect a device, or install an Android Emulator and system image."}
	}
	if requirements.CompileSDK == 0 {
		return android.SDK{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "A default Android emulator requires the project's numeric compileSdk version.", Remediation: "Declare compileSdk in Gradle, then retry or choose a connected device."}
	}
	if !acceptLicenses {
		return android.SDK{}, &codedError{Code: "MOB_LICENSE_REQUIRED", Message: "An Android Emulator and system image are required before Mob can create a device.", Remediation: "Review the Android SDK license, then repeat with --accept-licenses."}
	}
	images, err := android.ListAvailableSystemImages(ctx, sdk.Path)
	if err != nil {
		return android.SDK{}, &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Restore Android repository access and retry."}
	}
	image, found := defaultSystemImagePackage(images, requirements.CompileSDK)
	if !found {
		return android.SDK{}, &codedError{Code: "MOB_PACKAGE_NOT_AVAILABLE", Message: fmt.Sprintf("No Google APIs system image for Android API %d is available on this host.", requirements.CompileSDK), Remediation: "Run mob android emulator image available --api <level> and install a supported system image."}
	}
	catalog, err := android.LoadCatalog(ctx, android.CatalogCachePath(r.home), false)
	if err != nil {
		return android.SDK{}, &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Restore Android repository access and retry."}
	}
	config, err := r.store.Load()
	if err != nil {
		return android.SDK{}, err
	}
	if sdk.Ownership != state.OwnershipManaged {
		sdks, err := android.Discover(config)
		if err != nil {
			return android.SDK{}, err
		}
		sdk, err = r.installTarget("managed", sdks)
		if err != nil {
			return android.SDK{}, err
		}
	}
	packages := append(requiredAndroidPackages(requirements, true), "emulator", image)
	packages = uniquePackages(packages)
	for _, packageID := range packages {
		if packageID == image {
			continue
		}
		if _, found := catalog.FindPackage(packageID); !found {
			return android.SDK{}, &codedError{Code: "MOB_PACKAGE_NOT_AVAILABLE", Message: "Android package " + packageID + " is not in the current catalog.", Remediation: "Run mob android sdk available --refresh and retry."}
		}
	}
	if err := r.emit("progress", command, true, map[string]interface{}{"phase": "prepare-emulator", "sdk": sdk.Name, "packages": packages}, nil); err != nil {
		return android.SDK{}, err
	}
	if err := r.bootstrapManagedSDK(ctx, sdk, catalog, command); err != nil {
		return android.SDK{}, err
	}
	if _, err := android.InstallPackages(ctx, android.InstallRequest{Root: sdk.Path, Packages: packages, AcceptLicenses: true, Environment: androidProxyEnvironment(config), Output: r.sdkManagerOutput()}); err != nil {
		return android.SDK{}, &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Check Android repository access and the accepted SDK licenses, then retry."}
	}
	r.registerManagedSDK(&config, sdk)
	if err := r.store.Save(config); err != nil {
		return android.SDK{}, err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return android.SDK{}, err
	}
	for _, updated := range sdks {
		if updated.Name == sdk.Name {
			return updated, nil
		}
	}
	return android.SDK{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "The installed Android Emulator could not be discovered.", Remediation: "Run mob android sdk inspect managed."}
}

func matchingAndroidSDK(sdks []android.SDK, requirements project.AndroidRequirements, requireDevice bool) (android.SDK, bool) {
	for _, sdk := range sdks {
		if sdk.Current && sdkHasAndroidRequirements(sdk, requirements, requireDevice) {
			return sdk, true
		}
	}
	for _, sdk := range sdks {
		if sdkHasAndroidRequirements(sdk, requirements, requireDevice) {
			return sdk, true
		}
	}
	return android.SDK{}, false
}

func sdkHasAndroidRequirements(sdk android.SDK, requirements project.AndroidRequirements, requireDevice bool) bool {
	if requirements.CompileSDK > 0 && !containsString(sdk.Components.Platforms, fmt.Sprintf("android-%d", requirements.CompileSDK)) {
		return false
	}
	if requirements.BuildTools != "" && !containsString(sdk.Components.BuildTools, requirements.BuildTools) {
		return false
	}
	if requirements.NDKVersion != "" && !containsString(sdk.Components.NDK, requirements.NDKVersion) {
		return false
	}
	return !requireDevice || sdk.Components.PlatformTools
}

func requiredAndroidPackages(requirements project.AndroidRequirements, requireDevice bool) []string {
	packages := make([]string, 0, 4)
	if requirements.CompileSDK > 0 {
		packages = append(packages, fmt.Sprintf("platforms;android-%d", requirements.CompileSDK))
	}
	if requirements.BuildTools != "" {
		packages = append(packages, "build-tools;"+requirements.BuildTools)
	}
	if requirements.NDKVersion != "" {
		packages = append(packages, "ndk;"+requirements.NDKVersion)
	}
	if requireDevice {
		packages = append(packages, "platform-tools")
	}
	return packages
}

func (r runtime) validateCatalogPackages(ctx context.Context, packages []string) (android.Catalog, error) {
	catalog, err := android.LoadCatalog(ctx, android.CatalogCachePath(r.home), false)
	if err != nil {
		return android.Catalog{}, &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Run mob android sdk available --refresh after restoring Android repository access."}
	}
	for _, packageID := range packages {
		if _, found := catalog.FindPackage(packageID); !found {
			return android.Catalog{}, &codedError{Code: "MOB_PACKAGE_NOT_AVAILABLE", Message: "Android package " + packageID + " is not in the current catalog.", Remediation: "Run mob android sdk available --refresh and choose a package returned by the catalog."}
		}
	}
	return catalog, nil
}

func defaultSystemImagePackage(items []android.CatalogItem, api int) (string, bool) {
	prefix := fmt.Sprintf("system-images;android-%d;google_apis;", api)
	preferredABI := "x86_64"
	if goruntime.GOARCH == "arm64" {
		preferredABI = "arm64-v8a"
	}
	var fallback string
	for _, item := range items {
		if !strings.HasPrefix(item.PackageID, prefix) {
			continue
		}
		if strings.HasSuffix(item.PackageID, ";"+preferredABI) {
			return item.PackageID, true
		}
		if fallback == "" {
			fallback = item.PackageID
		}
	}
	return fallback, fallback != ""
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func uniquePackages(packages []string) []string {
	seen := make(map[string]bool, len(packages))
	result := make([]string, 0, len(packages))
	for _, packageID := range packages {
		if packageID == "" || seen[packageID] {
			continue
		}
		seen[packageID] = true
		result = append(result, packageID)
	}
	return result
}

func hasSystemImageForAPI(sdk android.SDK, api int) bool {
	if api == 0 {
		return len(sdk.Components.SystemImages) > 0
	}
	_, found := android.SystemImageForAPI(sdk, api)
	return found
}
