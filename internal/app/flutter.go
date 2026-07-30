package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"

	"github.com/xy200303/MobBase/internal/home"
	flutterplatform "github.com/xy200303/MobBase/internal/platform/flutter"
	"github.com/xy200303/MobBase/internal/state"
	"github.com/xy200303/MobBase/internal/system"
)

type flutterTool struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path"`
}

func (r runtime) flutter(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "create" {
		return r.flutterCreate(ctx, args[1:])
	}
	if len(args) > 0 && args[0] == "available" {
		return r.flutterAvailable(ctx, args[1:])
	}
	if len(args) > 0 && args[0] == "install" {
		return r.flutterInstall(ctx, args[1:])
	}
	if len(args) > 0 && args[0] == "use" {
		return r.flutterUse(args[1:])
	}
	if len(args) > 0 && args[0] == "remove" {
		return r.flutterRemove(args[1:])
	}
	if len(args) != 1 || args[0] != "list" {
		return invalidCommand("mob flutter " + joinCommand(args))
	}
	tools := make([]flutterTool, 0, 2)
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	for _, sdk := range config.Flutter.SDKs {
		tools = append(tools, flutterTool{ID: "flutter:managed:" + sdk.Version, Kind: "managed", Path: sdk.Path})
	}
	if executable, found := system.LookPath("flutter"); found {
		tools = append(tools, flutterTool{ID: "flutter:system", Kind: "flutter", Path: executable})
	}
	if executable, found := system.LookPath("fvm"); found {
		tools = append(tools, flutterTool{ID: "flutter:fvm", Kind: "fvm", Path: executable})
	}
	if r.json {
		return r.result("mob flutter list", map[string]interface{}{"tools": tools})
	}
	if len(tools) == 0 {
		fmt.Fprintln(r.out, "No Flutter or FVM launcher found on PATH.")
		return nil
	}
	rows := make([][]string, 0, len(tools))
	for _, tool := range tools {
		rows = append(rows, []string{tool.ID, tool.Path})
	}
	if !r.terminal.Table([]string{"ID", "PATH"}, rows) {
		for _, row := range rows {
			fmt.Fprintf(r.out, "%s\t%s\n", row[0], row[1])
		}
	}
	return nil
}

func (r runtime) flutterInstall(ctx context.Context, args []string) error {
	version := ""
	if len(args) > 0 {
		if len(args) != 2 || args[0] != "--version" || strings.TrimSpace(args[1]) == "" {
			return invalidCommand("mob flutter install " + strings.Join(args, " "))
		}
		version = args[1]
	}
	if err := r.emit("started", "mob flutter install", true, map[string]string{"phase": "install", "version": versionOrStable(version)}, nil); err != nil {
		return err
	}
	sdk, checksum, err := r.installFlutter(ctx, version, "mob flutter install")
	if err != nil {
		return err
	}
	data := map[string]interface{}{"version": sdk.Version, "path": sdk.Path, "sha256": checksum}
	if r.json {
		return r.result("mob flutter install", data)
	}
	fmt.Fprintf(r.out, "Installed Flutter %s.\n", sdk.Version)
	return nil
}

func (r runtime) installFlutter(ctx context.Context, version, command string) (state.FlutterSDK, string, error) {
	catalog, err := flutterplatform.LoadCatalog(ctx, flutterplatform.CachePath(r.home), false)
	if err != nil {
		return state.FlutterSDK{}, "", &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Run mob flutter available --refresh after restoring access to the Flutter official release directory."}
	}
	var selected *flutterplatform.Release
	for index := range catalog.Releases {
		item := &catalog.Releases[index]
		if (version != "" && item.Version == version) || (version == "" && item.Current) {
			selected = item
			break
		}
	}
	if selected == nil && version == "" && len(catalog.Releases) > 0 {
		selected = &catalog.Releases[0]
	}
	if selected == nil {
		return state.FlutterSDK{}, "", &codedError{Code: "MOB_PACKAGE_NOT_AVAILABLE", Message: "Requested Flutter version is not in the current official catalog."}
	}
	destination := filepath.Join(r.home, "toolchains", "flutter", selected.Version)
	config, err := r.store.Load()
	if err != nil {
		return state.FlutterSDK{}, "", err
	}
	for _, sdk := range config.Flutter.SDKs {
		if sdk.Version == selected.Version && samePath(sdk.Path, destination) {
			if _, statErr := os.Stat(destination); statErr == nil {
				return sdk, selected.SHA256, nil
			}
		}
	}
	if err := r.emit("progress", command, true, map[string]string{"phase": "download-flutter", "version": selected.Version}, nil); err != nil {
		return state.FlutterSDK{}, "", err
	}
	var report func(flutterplatform.DownloadProgress)
	if callback := r.download("Downloading Flutter SDK"); callback != nil {
		report = func(progress flutterplatform.DownloadProgress) {
			callback(progress.Downloaded, progress.Total)
		}
	}
	if err := flutterplatform.Install(ctx, destination, filepath.Join(r.home, "cache", "downloads"), *selected, report); err != nil {
		return state.FlutterSDK{}, "", &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Check Flutter release access and retry; existing installations are not modified."}
	}
	sdk := state.FlutterSDK{Version: selected.Version, Path: destination}
	config.Flutter.SDKs = append(config.Flutter.SDKs, sdk)
	if config.Flutter.CurrentSDK == "" {
		config.Flutter.CurrentSDK = selected.Version
	}
	if err := r.store.Save(config); err != nil {
		return state.FlutterSDK{}, "", err
	}
	return sdk, selected.SHA256, nil
}

func versionOrStable(version string) string {
	if version == "" {
		return "stable"
	}
	return version
}

func samePath(left, right string) bool {
	left, leftErr := filepath.Abs(filepath.Clean(left))
	right, rightErr := filepath.Abs(filepath.Clean(right))
	return leftErr == nil && rightErr == nil && strings.EqualFold(left, right)
}

func (r runtime) flutterUse(args []string) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Flutter version is required.", Remediation: "Run mob flutter list, then use mob flutter use <version>."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	var selected *state.FlutterSDK
	for index := range config.Flutter.SDKs {
		if config.Flutter.SDKs[index].Version == args[0] {
			selected = &config.Flutter.SDKs[index]
			break
		}
	}
	if selected == nil {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Mob-managed Flutter " + args[0] + " is not installed.", Remediation: "Run mob flutter available, then mob flutter install --version " + args[0] + "."}
	}
	executable := filepath.Join(selected.Path, "bin", "flutter")
	if goruntime.GOOS == "windows" {
		executable += ".bat"
	}
	if info, err := os.Stat(executable); err != nil || info.IsDir() {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Mob-managed Flutter executable is missing: " + executable, Remediation: "Reinstall the selected Flutter version."}
	}
	config.Flutter.CurrentSDK = selected.Version
	if err := r.store.Save(config); err != nil {
		return err
	}
	data := map[string]string{"version": selected.Version, "path": selected.Path}
	if r.json {
		return r.result("mob flutter use", data)
	}
	fmt.Fprintf(r.out, "Current Flutter SDK: %s\n", selected.Version)
	return nil
}

func (r runtime) flutterRemove(args []string) error {
	if len(args) != 2 || args[1] != "--yes" || strings.TrimSpace(args[0]) == "" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Removing Flutter requires <version> and --yes.", Remediation: "Run mob flutter list, then use mob flutter remove <version> --yes."}
	}
	version := args[0]
	if filepath.Base(version) != version || version == "." {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Flutter version must not contain a path separator."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	index := -1
	for current, sdk := range config.Flutter.SDKs {
		if sdk.Version == version {
			index = current
			break
		}
	}
	if index < 0 {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Mob-managed Flutter " + version + " is not installed."}
	}
	path, err := filepath.Abs(filepath.Clean(config.Flutter.SDKs[index].Path))
	if err != nil {
		return err
	}
	expected, err := filepath.Abs(filepath.Join(r.home, "toolchains", "flutter", version))
	if err != nil {
		return err
	}
	if !strings.EqualFold(path, expected) {
		return &codedError{Code: "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", Message: "Only Flutter versions inside Mob's managed toolchain directory can be removed."}
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove Mob-managed Flutter: %w", err)
	}
	config.Flutter.SDKs = append(config.Flutter.SDKs[:index], config.Flutter.SDKs[index+1:]...)
	if config.Flutter.CurrentSDK == version {
		config.Flutter.CurrentSDK = ""
	}
	if err := r.store.Save(config); err != nil {
		return err
	}
	data := map[string]string{"version": version, "path": path}
	if r.json {
		return r.result("mob flutter remove", data)
	}
	fmt.Fprintf(r.out, "Removed Flutter %s.\n", version)
	return nil
}

func (r runtime) flutterAvailable(ctx context.Context, args []string) error {
	refresh, err := parseRefresh(args, "mob flutter available")
	if err != nil {
		return err
	}
	catalog, err := flutterplatform.LoadCatalog(ctx, flutterplatform.CachePath(r.home), refresh)
	if err != nil {
		return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Check access to the Flutter official release directory, or retry with a cached catalog."}
	}
	data := map[string]interface{}{"source": catalog.Source, "refreshedAt": catalog.RefreshedAt, "cached": catalog.Cached, "releases": catalog.Releases}
	if r.json {
		return r.result("mob flutter available", data)
	}
	rows := make([][]string, 0, len(catalog.Releases))
	for _, release := range catalog.Releases {
		current := ""
		if release.Current {
			current = "current"
		}
		rows = append(rows, []string{release.Version, release.Archive, release.SHA256, current})
	}
	if !r.terminal.Table([]string{"VERSION", "ARCHIVE", "SHA256", "CURRENT"}, rows) {
		for _, row := range rows {
			current := ""
			if row[3] != "" {
				current = "\t" + row[3]
			}
			fmt.Fprintf(r.out, "%s\t%s\t%s%s\n", row[0], row[1], row[2], current)
		}
	}
	return nil
}

var flutterProjectName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func (r runtime) flutterCreate(ctx context.Context, args []string) error {
	name, platforms, err := parseFlutterCreate(args)
	if err != nil {
		return err
	}
	if err := r.emit("started", "mob flutter create", true, map[string]string{"phase": "create", "project": name}, nil); err != nil {
		return err
	}
	runner, err := r.ensureFlutterRunner(ctx, "", false, "mob flutter create")
	if err != nil {
		return err
	}
	arguments := append([]string{}, runner.Prefix...)
	arguments = append(arguments, "create")
	if platforms != "" {
		arguments = append(arguments, "--platforms="+platforms)
	}
	arguments = append(arguments, name)
	program, arguments := system.BatchCommand(runner.Program, arguments...)
	result, commandErr := r.executeWorkflowCommand(ctx, "mob flutter create", program, arguments, nil, "")
	if result.Output != "" {
		if r.json {
			if err := r.emit("log", "mob flutter create", true, map[string]string{"stream": "combined", "output": result.Output}, nil); err != nil {
				return err
			}
		} else {
			fmt.Fprint(r.out, result.Output)
		}
	}
	if commandErr != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "Flutter project creation failed: " + commandErr.Error(), Remediation: "Review the Flutter output and requested platform list."}
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	path := filepath.Join(workingDirectory, name)
	data := map[string]string{"project": name, "path": path, "runner": runner.Program}
	if r.json {
		return r.result("mob flutter create", data)
	}
	fmt.Fprintf(r.out, "Created Flutter project %s.\n", path)
	return nil
}

func parseFlutterCreate(args []string) (string, string, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") || !flutterProjectName.MatchString(args[0]) {
		return "", "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Flutter project name must be a lowercase Dart package name.", Remediation: "Use mob flutter create <name>, for example mob flutter create travel_app."}
	}
	name := args[0]
	platforms := ""
	for args = args[1:]; len(args) > 0; {
		if args[0] != "--platforms" || len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return "", "", invalidCommand("mob flutter create " + strings.Join(append([]string{name}, args...), " "))
		}
		platforms = args[1]
		for _, platform := range strings.Split(platforms, ",") {
			if platform != "android" && platform != "ios" {
				return "", "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--platforms supports android and ios."}
			}
		}
		args = args[2:]
	}
	return name, platforms, nil
}

func joinCommand(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return strings.Join(args, " ")
}

type flutterCommand struct {
	Program string
	Prefix  []string
}

// flutterRunner follows the project-owned FVM marker without reading or
// changing its version data. A regular Flutter project uses the discovered
// official flutter executable.
func flutterRunner(root string) (flutterCommand, error) {
	return flutterRunnerWithLookup(root, system.LookPath)
}

// ensureFlutterRunner installs only the official Flutter SDK needed by a
// regular project. Projects with .fvmrc remain owned by FVM and are never
// changed or inferred by Mob.
func (r runtime) ensureFlutterRunner(ctx context.Context, root string, noInstall bool, command string) (flutterCommand, error) {
	if _, err := os.Stat(filepath.Join(root, ".fvmrc")); err == nil {
		return r.ensureFVMRunner(ctx, noInstall, command)
	}
	return r.ensureRegularFlutterRunner(ctx, noInstall, command)
}

func (r runtime) ensureRegularFlutterRunner(ctx context.Context, noInstall bool, command string) (flutterCommand, error) {
	runner, err := regularFlutterRunner()
	if err == nil {
		return runner, nil
	}
	if noInstall {
		return flutterCommand{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Flutter was not found and automatic installation is disabled.", Remediation: "Run mob flutter install, or rerun without --no-install."}
	}
	if _, _, installErr := r.installFlutter(ctx, "", command); installErr != nil {
		return flutterCommand{}, installErr
	}
	return regularFlutterRunner()
}

func flutterRunnerWithLookup(root string, lookup func(string) (string, bool)) (flutterCommand, error) {
	if _, err := os.Stat(filepath.Join(root, ".fvmrc")); err == nil {
		if executable, found := lookup("fvm"); found {
			return flutterCommand{Program: executable, Prefix: []string{"flutter"}}, nil
		}
		if homePath, homeErr := home.Resolve(); homeErr == nil {
			if config, loadErr := state.New(homePath).Load(); loadErr == nil {
				if executable, found := managedFVMLauncher(config); found {
					return flutterCommand{Program: executable, Prefix: []string{"flutter"}}, nil
				}
			}
		}
		return flutterCommand{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "This Flutter project uses FVM but the fvm launcher was not found.", Remediation: "Install FVM with its official instructions, then rerun mob; Mob does not read or modify .fvmrc."}
	}
	return regularFlutterRunnerWithLookup(lookup)
}

func regularFlutterRunner() (flutterCommand, error) {
	return regularFlutterRunnerWithLookup(system.LookPath)
}

func regularFlutterRunnerWithLookup(lookup func(string) (string, bool)) (flutterCommand, error) {
	if homePath, err := home.Resolve(); err == nil {
		if config, loadErr := state.New(homePath).Load(); loadErr == nil && config.Flutter.CurrentSDK != "" {
			for _, sdk := range config.Flutter.SDKs {
				if sdk.Version != config.Flutter.CurrentSDK {
					continue
				}
				executable := filepath.Join(sdk.Path, "bin", "flutter")
				if goruntime.GOOS == "windows" {
					executable += ".bat"
				}
				if path, found := lookup(executable); found {
					return flutterCommand{Program: path}, nil
				}
			}
		}
	}
	if executable, found := lookup("flutter"); found {
		return flutterCommand{Program: executable}, nil
	}
	return flutterCommand{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Flutter was not found on PATH.", Remediation: "Install Flutter through its official distribution, then rerun mob."}
}

func flutterBuildCommand(root string, forwarded []string) (string, []string, error) {
	if len(forwarded) > 0 {
		return forwarded[0], forwarded[1:], nil
	}
	runner, err := flutterRunner(root)
	if err != nil {
		return "", nil, err
	}
	return runner.Program, append(runner.Prefix, "build", "apk"), nil
}

func flutterRunCommand(root string, forwarded []string, nativeDeviceID string) (string, []string, error) {
	if len(forwarded) > 0 {
		return forwarded[0], forwarded[1:], nil
	}
	runner, err := flutterRunner(root)
	if err != nil {
		return "", nil, err
	}
	arguments := append([]string{}, runner.Prefix...)
	arguments = append(arguments, "run", "-d", nativeDeviceID)
	return runner.Program, arguments, nil
}
