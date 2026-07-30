package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"

	javaplatform "github.com/xy200303/MobBase/internal/platform/java"
	"github.com/xy200303/MobBase/internal/state"
	"github.com/xy200303/MobBase/internal/system"
)

func (r runtime) java(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return invalidCommand("mob java")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return invalidCommand("mob java " + strings.Join(args, " "))
		}
		return r.javaList(ctx)
	case "import":
		return r.javaImport(ctx, args[1:])
	case "available":
		return r.javaAvailable(ctx, args[1:])
	case "install":
		return r.javaInstall(ctx, args[1:])
	case "remove":
		return r.javaRemove(args[1:])
	case "use":
		return r.javaUse(args[1:])
	default:
		return invalidCommand("mob java " + strings.Join(args, " "))
	}
}

func (r runtime) javaRemove(args []string) error {
	if len(args) != 2 || args[1] != "--yes" || strings.TrimSpace(args[0]) == "" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Removing a JDK requires <name> and --yes.", Remediation: "Run mob java list, then use mob java remove <name> --yes."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	index := -1
	for current, sdk := range config.Java.SDKs {
		if sdk.Name == args[0] {
			index = current
			break
		}
	}
	if index < 0 {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "No registered JDK named " + args[0] + " was found."}
	}
	sdk := config.Java.SDKs[index]
	expected := filepath.Join(r.home, "toolchains", "java", filepath.Base(sdk.Path))
	if sdk.Ownership != state.OwnershipManaged || !samePath(sdk.Path, expected) {
		return &codedError{Code: "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", Message: "Only JDKs inside Mob's managed toolchain directory can be removed."}
	}
	if err := os.RemoveAll(sdk.Path); err != nil {
		return fmt.Errorf("remove Mob-managed JDK: %w", err)
	}
	config.Java.SDKs = append(config.Java.SDKs[:index], config.Java.SDKs[index+1:]...)
	if config.Java.CurrentSDK == sdk.Name {
		config.Java.CurrentSDK = ""
	}
	if err := r.store.Save(config); err != nil {
		return err
	}
	data := map[string]string{"name": sdk.Name, "path": sdk.Path}
	if r.json {
		return r.result("mob java remove", data)
	}
	fmt.Fprintf(r.out, "Removed JDK %s.\n", sdk.Name)
	return nil
}

func (r runtime) javaInstall(ctx context.Context, args []string) error {
	if len(args) != 1 || !regexp.MustCompile(`^(8|11|17|21)$`).MatchString(args[0]) {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "JDK install requires one supported major version: 8, 11, 17, or 21."}
	}
	major, _ := strconv.Atoi(args[0])
	catalog, err := javaplatform.LoadCatalog(ctx, javaplatform.CachePath(r.home), false)
	if err != nil {
		return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error()}
	}
	var release *javaplatform.Release
	for index := range catalog.Releases {
		if catalog.Releases[index].Major == major {
			release = &catalog.Releases[index]
			break
		}
	}
	if release == nil {
		return &codedError{Code: "MOB_PACKAGE_NOT_AVAILABLE", Message: "Requested JDK is not in the official catalog."}
	}
	destination := filepath.Join(r.home, "toolchains", "java", release.Version)
	if err := r.emit("started", "mob java install", true, map[string]string{"phase": "install", "version": release.Version}, nil); err != nil {
		return err
	}
	var report func(javaplatform.DownloadProgress)
	if callback := r.download("Downloading Temurin JDK"); callback != nil {
		report = func(progress javaplatform.DownloadProgress) {
			callback(progress.Downloaded, progress.Total)
		}
	}
	if err := javaplatform.Install(ctx, destination, filepath.Join(r.home, "cache", "downloads"), *release, report); err != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Check access to the official Temurin archive and retry."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	name := "temurin-" + strconv.Itoa(major)
	config.Java.SDKs = append(config.Java.SDKs, state.JavaSDK{Name: name, Version: major, Path: destination, Ownership: state.OwnershipManaged})
	if config.Java.CurrentSDK == "" {
		config.Java.CurrentSDK = name
	}
	if err := r.store.Save(config); err != nil {
		return err
	}
	data := map[string]interface{}{"name": name, "version": major, "path": destination, "sha256": release.SHA256}
	if r.json {
		return r.result("mob java install", data)
	}
	fmt.Fprintf(r.out, "Installed JDK %d at %s.\n", major, destination)
	return nil
}

func (r runtime) javaAvailable(ctx context.Context, args []string) error {
	refresh, err := parseRefresh(args, "mob java available")
	if err != nil {
		return err
	}
	catalog, err := javaplatform.LoadCatalog(ctx, javaplatform.CachePath(r.home), refresh)
	if err != nil {
		return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Check access to the Eclipse Temurin official API, then retry with --refresh."}
	}
	if r.json {
		return r.result("mob java available", catalog)
	}
	rows := make([][]string, 0, len(catalog.Releases))
	for _, release := range catalog.Releases {
		rows = append(rows, []string{strconv.Itoa(release.Major), release.Version, release.SHA256})
	}
	if !r.terminal.Table([]string{"MAJOR", "VERSION", "SHA256"}, rows) {
		for _, row := range rows {
			fmt.Fprintf(r.out, "%s\t%s\t%s\n", row[0], row[1], row[2])
		}
	}
	return nil
}

func (r runtime) javaList(ctx context.Context) error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	sdks, err := discoverJava(ctx, config)
	if err != nil {
		return err
	}
	data := map[string]interface{}{"sdks": sdks, "currentSdk": config.Java.CurrentSDK}
	if r.json {
		return r.result("mob java list", data)
	}
	if len(sdks) == 0 {
		fmt.Fprintln(r.out, "No JDK was found. Import one with mob java import --path <jdk-root> --name <name>.")
		return nil
	}
	rows := make([][]string, 0, len(sdks))
	for _, sdk := range sdks {
		current := ""
		if sdk.Name == config.Java.CurrentSDK {
			current = "current"
		}
		rows = append(rows, []string{sdk.Name, strconv.Itoa(sdk.Version), string(sdk.Ownership), sdk.Path, current})
	}
	if !r.terminal.Table([]string{"NAME", "VERSION", "OWNERSHIP", "PATH", "CURRENT"}, rows) {
		for _, row := range rows {
			current := ""
			if row[4] != "" {
				current = "\t" + row[4]
			}
			fmt.Fprintf(r.out, "%s\t%s\t%s\t%s%s\n", row[0], row[1], row[2], row[3], current)
		}
	}
	return nil
}

func (r runtime) javaImport(ctx context.Context, args []string) error {
	path, name, err := parseJavaImport(args)
	if err != nil {
		return err
	}
	version, err := javaVersion(ctx, path)
	if err != nil {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: err.Error(), Remediation: "Pass the JDK root directory containing bin/java."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	for _, sdk := range config.Java.SDKs {
		if sdk.Name == name {
			return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "A JDK named " + name + " is already registered.", Remediation: "Choose a different --name, or remove the existing registration first."}
		}
	}
	sdk := state.JavaSDK{Name: name, Version: version, Path: path, Ownership: state.OwnershipImported}
	config.Java.SDKs = append(config.Java.SDKs, sdk)
	if err := r.store.Save(config); err != nil {
		return err
	}
	if r.json {
		return r.result("mob java import", sdk)
	}
	fmt.Fprintf(r.out, "Imported JDK %s (Java %d).\n", name, version)
	return nil
}

func parseJavaImport(args []string) (string, string, error) {
	var path, name string
	for len(args) > 0 {
		if len(args) < 2 || (args[0] != "--path" && args[0] != "--name") || strings.TrimSpace(args[1]) == "" {
			return "", "", invalidCommand("mob java import " + strings.Join(args, " "))
		}
		if args[0] == "--path" {
			path = args[1]
		} else {
			name = args[1]
		}
		args = args[2:]
	}
	if path == "" || name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `\\/:`) {
		return "", "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Java import requires --path <jdk-root> and a simple --name <name>."}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	return abs, name, nil
}

func (r runtime) javaUse(args []string) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "A registered JDK name is required.", Remediation: "Run mob java list, then use mob java use <name>."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	for _, sdk := range config.Java.SDKs {
		if sdk.Name != args[0] {
			continue
		}
		config.Java.CurrentSDK = sdk.Name
		if err := r.store.Save(config); err != nil {
			return err
		}
		if r.json {
			return r.result("mob java use", sdk)
		}
		fmt.Fprintf(r.out, "Current Mob JDK: %s (Java %d).\n", sdk.Name, sdk.Version)
		return nil
	}
	return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "No registered JDK named " + args[0] + " was found.", Remediation: "Run mob java list, or import a JDK with mob java import."}
}

func discoverJava(ctx context.Context, config state.Config) ([]state.JavaSDK, error) {
	result := make([]state.JavaSDK, 0, len(config.Java.SDKs))
	result = append(result, config.Java.SDKs...)
	seen := map[string]bool{}
	for _, sdk := range result {
		seen[normalizedPath(sdk.Path)] = true
	}
	for _, candidate := range []string{os.Getenv("JAVA_HOME"), javaHomeFromPath()} {
		if candidate == "" || seen[normalizedPath(candidate)] {
			continue
		}
		version, err := javaVersion(ctx, candidate)
		if err != nil {
			continue
		}
		name := "system-java"
		if os.Getenv("JAVA_HOME") == candidate {
			name = "java-home"
		}
		result = append(result, state.JavaSDK{Name: name, Version: version, Path: candidate, Ownership: state.OwnershipDiscovered})
		seen[normalizedPath(candidate)] = true
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func javaHomeFromPath() string {
	java, found := system.LookPath("java")
	if !found {
		return ""
	}
	return filepath.Dir(filepath.Dir(java))
}

func javaVersion(ctx context.Context, home string) (int, error) {
	java := filepath.Join(home, "bin", "java")
	if goruntime.GOOS == "windows" {
		java += ".exe"
	}
	if info, err := os.Stat(java); err != nil || info.IsDir() {
		return 0, fmt.Errorf("JDK executable was not found: %s", java)
	}
	result, err := system.Run(ctx, java, []string{"-version"}, nil, "", "")
	if err != nil {
		return 0, fmt.Errorf("read JDK version: %w", err)
	}
	versionPattern := regexp.MustCompile(`(?m)(?:java|openjdk) version "(?:1\.)?(\d+)`)
	match := versionPattern.FindStringSubmatch(result.Output)
	if len(match) != 2 {
		return 0, fmt.Errorf("could not parse JDK version from %s", java)
	}
	version, err := strconv.Atoi(match[1])
	if err != nil || version == 0 {
		return 0, fmt.Errorf("could not parse JDK version from %s", java)
	}
	return version, nil
}

func normalizedPath(path string) string {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return path
	}
	return strings.ToLower(abs)
}

func (r runtime) selectProjectJava(ctx context.Context, required int, noInstall bool) (state.JavaSDK, error) {
	config, err := r.store.Load()
	if err != nil {
		return state.JavaSDK{}, err
	}
	sdks, err := discoverJava(ctx, config)
	if err != nil {
		return state.JavaSDK{}, err
	}
	if config.Java.CurrentSDK != "" {
		for _, sdk := range sdks {
			if sdk.Name == config.Java.CurrentSDK && (required == 0 || sdk.Version == required) {
				return sdk, nil
			}
		}
	}
	for _, sdk := range sdks {
		if required == 0 || sdk.Version == required {
			return sdk, nil
		}
	}
	requirement := "a usable JDK"
	if required > 0 {
		requirement = fmt.Sprintf("JDK %d", required)
	}
	if noInstall {
		return state.JavaSDK{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "No " + requirement + " is available and automatic installation is disabled.", Remediation: "Run mob java install " + strconv.Itoa(defaultJavaVersion(required)) + ", or rerun without --no-install."}
	}
	if err := r.javaInstall(ctx, []string{strconv.Itoa(defaultJavaVersion(required))}); err != nil {
		return state.JavaSDK{}, err
	}
	return r.selectProjectJava(ctx, required, true)
}

func defaultJavaVersion(required int) int {
	if required != 0 {
		return required
	}
	return 17
}

func javaEnvironment(sdk state.JavaSDK) []string {
	bin := filepath.Join(sdk.Path, "bin")
	path := os.Getenv("PATH")
	return []string{"JAVA_HOME=" + sdk.Path, "PATH=" + bin + string(os.PathListSeparator) + path}
}
