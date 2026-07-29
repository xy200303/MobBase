package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/fvm"
	"github.com/xy200303/MobBase/internal/project"
	"github.com/xy200303/MobBase/internal/state"
	"github.com/xy200303/MobBase/internal/system"
)

func (r runtime) fvm(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return invalidCommand("mob fvm")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return invalidCommand("mob fvm " + strings.Join(args, " "))
		}
		return r.fvmList()
	case "status":
		if len(args) != 1 {
			return invalidCommand("mob fvm " + strings.Join(args, " "))
		}
		return r.fvmStatus()
	case "available":
		return r.fvmAvailable(ctx, args[1:])
	case "install":
		return r.fvmInstall(ctx, args[1:], "mob fvm install", false)
	case "update":
		if len(args) != 1 {
			return invalidCommand("mob fvm update " + strings.Join(args[1:], " "))
		}
		return r.fvmInstall(ctx, nil, "mob fvm update", true)
	case "use":
		return r.fvmUse(args[1:])
	case "remove":
		return r.fvmRemove(args[1:])
	default:
		return invalidCommand("mob fvm " + strings.Join(args, " "))
	}
}

func (r runtime) fvmList() error {
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	items := make([]map[string]interface{}, 0, len(config.FVM.SDKs))
	for _, sdk := range config.FVM.SDKs {
		items = append(items, map[string]interface{}{"version": sdk.Version, "path": sdk.Path, "sha256": sdk.SHA256, "current": sdk.Version == config.FVM.CurrentSDK, "ready": launcherExists(sdk.Path)})
	}
	if r.json {
		return r.result("mob fvm list", map[string]interface{}{"versions": items, "currentVersion": config.FVM.CurrentSDK})
	}
	if len(items) == 0 {
		fmt.Fprintln(r.out, "No Mob-managed FVM version is installed.")
		return nil
	}
	for _, item := range items {
		marker := ""
		if item["current"].(bool) {
			marker = "\tcurrent"
		}
		fmt.Fprintf(r.out, "%s\t%s%s\n", item["version"], item["path"], marker)
	}
	return nil
}

func (r runtime) fvmStatus() error {
	systemLauncher, systemAvailable := system.LookPath("fvm")
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	managedLauncher, managedAvailable := managedFVMLauncher(config)
	currentProject, err := project.Detect("")
	if err != nil {
		return err
	}
	marker := false
	if currentProject != nil && currentProject.Kind == project.KindFlutter {
		_, markerErr := os.Stat(filepath.Join(currentProject.Root, ".fvmrc"))
		marker = markerErr == nil
	}
	data := map[string]interface{}{"launcherAvailable": systemAvailable || managedAvailable, "launcher": firstNonEmpty(systemLauncher, managedLauncher), "systemLauncher": systemLauncher, "managedLauncher": managedLauncher, "managedVersion": config.FVM.CurrentSDK, "projectUsesFVM": marker}
	if r.json {
		return r.result("mob fvm status", data)
	}
	if systemAvailable {
		fmt.Fprintf(r.out, "System FVM launcher: %s\n", systemLauncher)
	}
	if managedAvailable {
		fmt.Fprintf(r.out, "Mob-managed FVM launcher: %s\n", managedLauncher)
	}
	if !systemAvailable && !managedAvailable {
		fmt.Fprintln(r.out, "FVM launcher: not found")
	}
	fmt.Fprintf(r.out, "Current project uses FVM: %t\n", marker)
	return nil
}

func (r runtime) fvmUse(args []string) error {
	if len(args) != 1 || !validToolVersion(args[0]) {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "FVM use requires one installed version.", Remediation: "Run mob fvm list, then use mob fvm use <version>."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	for _, sdk := range config.FVM.SDKs {
		if sdk.Version != args[0] {
			continue
		}
		if !launcherExists(sdk.Path) {
			return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Mob-managed FVM launcher is missing: " + fvmLauncherPath(sdk.Path), Remediation: "Reinstall this FVM version before selecting it."}
		}
		config.FVM.CurrentSDK = sdk.Version
		if err := r.store.Save(config); err != nil {
			return err
		}
		data := map[string]string{"version": sdk.Version, "path": sdk.Path}
		if r.json {
			return r.result("mob fvm use", data)
		}
		fmt.Fprintf(r.out, "Current FVM launcher: %s\n", sdk.Version)
		return nil
	}
	return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Mob-managed FVM " + args[0] + " is not installed.", Remediation: "Run mob fvm available, then mob fvm install --version " + args[0] + "."}
}

func (r runtime) fvmRemove(args []string) error {
	if len(args) != 2 || !validToolVersion(args[0]) || args[1] != "--yes" {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Removing FVM requires <version> and --yes.", Remediation: "Run mob fvm list, then use mob fvm remove <version> --yes."}
	}
	config, err := r.store.Load()
	if err != nil {
		return err
	}
	index := -1
	for current, sdk := range config.FVM.SDKs {
		if sdk.Version == args[0] {
			index = current
			break
		}
	}
	if index < 0 {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Mob-managed FVM " + args[0] + " is not installed."}
	}
	path, err := filepath.Abs(filepath.Clean(config.FVM.SDKs[index].Path))
	if err != nil {
		return err
	}
	expected, err := filepath.Abs(filepath.Join(r.home, "toolchains", "fvm", args[0]))
	if err != nil {
		return err
	}
	if !strings.EqualFold(path, expected) {
		return &codedError{Code: "MOB_EXTERNAL_TOOLCHAIN_WRITE_DENIED", Message: "Only FVM versions inside Mob's managed toolchain directory can be removed."}
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove Mob-managed FVM: %w", err)
	}
	config.FVM.SDKs = append(config.FVM.SDKs[:index], config.FVM.SDKs[index+1:]...)
	if config.FVM.CurrentSDK == args[0] {
		config.FVM.CurrentSDK = ""
	}
	if err := r.store.Save(config); err != nil {
		return err
	}
	data := map[string]string{"version": args[0], "path": path}
	if r.json {
		return r.result("mob fvm remove", data)
	}
	fmt.Fprintf(r.out, "Removed FVM %s.\n", args[0])
	return nil
}

func (r runtime) fvmAvailable(ctx context.Context, args []string) error {
	refresh, err := parseRefresh(args, "mob fvm available")
	if err != nil {
		return err
	}
	catalog, err := fvm.LoadCatalog(ctx, fvm.CachePath(r.home), refresh)
	if err != nil {
		return &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Check access to pub.dev, or retry with a cached FVM catalog."}
	}
	data := map[string]interface{}{"source": catalog.Source, "refreshedAt": catalog.RefreshedAt, "cached": catalog.Cached, "releases": catalog.Releases}
	if r.json {
		return r.result("mob fvm available", data)
	}
	for _, release := range catalog.Releases {
		current := ""
		if release.Current {
			current = "\tcurrent"
		}
		fmt.Fprintf(r.out, "%s\t%s%s\n", release.Version, release.SHA256, current)
	}
	return nil
}

func (r runtime) fvmInstall(ctx context.Context, args []string, command string, refresh bool) error {
	version, err := parseFVMVersion(args, command)
	if err != nil {
		return err
	}
	if err := r.emit("started", command, true, map[string]string{"phase": "install", "version": versionOrStable(version)}, nil); err != nil {
		return err
	}
	sdk, err := r.installFVM(ctx, version, command, refresh)
	if err != nil {
		return err
	}
	data := map[string]string{"version": sdk.Version, "path": sdk.Path, "sha256": sdk.SHA256}
	if r.json {
		return r.result(command, data)
	}
	fmt.Fprintf(r.out, "Installed FVM %s.\n", sdk.Version)
	return nil
}

func parseFVMVersion(args []string, command string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) != 2 || args[0] != "--version" || !validToolVersion(args[1]) {
		return "", &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "FVM version must be a dot-separated release version.", Remediation: "Use " + command + " [--version 4.1.2]."}
	}
	return args[1], nil
}

func validToolVersion(value string) bool {
	if value == "" || strings.ContainsAny(value, `\\/:`) {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return false
		}
		for _, char := range part {
			if (char < '0' || char > '9') && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && char != '-' {
				return false
			}
		}
	}
	return true
}

func (r runtime) installFVM(ctx context.Context, version, command string, refresh bool) (state.FVMSDK, error) {
	catalog, err := fvm.LoadCatalog(ctx, fvm.CachePath(r.home), refresh)
	if err != nil {
		return state.FVMSDK{}, &codedError{Code: "MOB_CATALOG_UNAVAILABLE", Message: err.Error(), Remediation: "Run mob fvm available --refresh after restoring access to pub.dev."}
	}
	var selected *fvm.Release
	for index := range catalog.Releases {
		item := &catalog.Releases[index]
		if (version != "" && item.Version == version) || (version == "" && item.Current) {
			selected = item
			break
		}
	}
	if selected == nil {
		return state.FVMSDK{}, &codedError{Code: "MOB_PACKAGE_NOT_AVAILABLE", Message: "Requested FVM version is not in the current official catalog."}
	}
	config, err := r.store.Load()
	if err != nil {
		return state.FVMSDK{}, err
	}
	for _, installed := range config.FVM.SDKs {
		if installed.Version == selected.Version && launcherExists(installed.Path) {
			config.FVM.CurrentSDK = installed.Version
			if err := r.store.Save(config); err != nil {
				return state.FVMSDK{}, err
			}
			return installed, nil
		}
	}
	runner, err := r.ensureRegularFlutterRunner(ctx, false, command)
	if err != nil {
		return state.FVMSDK{}, err
	}
	dart, found := dartForFlutter(runner.Program)
	if !found {
		return state.FVMSDK{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "A Dart executable was not found next to the Flutter runner.", Remediation: "Reinstall Flutter, then retry mob fvm install."}
	}
	destination := filepath.Join(r.home, "toolchains", "fvm", selected.Version)
	if _, err := os.Stat(destination); err == nil {
		return state.FVMSDK{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "FVM destination exists but its launcher is incomplete: " + destination, Remediation: "Remove the incomplete Mob-owned directory and rerun mob fvm install."}
	} else if !os.IsNotExist(err) {
		return state.FVMSDK{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return state.FVMSDK{}, err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(destination), ".fvm-install-")
	if err != nil {
		return state.FVMSDK{}, err
	}
	defer os.RemoveAll(temporary)
	source := filepath.Join(temporary, "source")
	if err := fvm.DownloadAndExtract(ctx, *selected, source); err != nil {
		return state.FVMSDK{}, &codedError{Code: "MOB_SOURCE_INVALID", Message: err.Error(), Remediation: "Refresh the FVM catalog and retry; Mob rejected an unverified package archive."}
	}
	pubCache := filepath.Join(temporary, "pub-cache")
	program, arguments := system.BatchCommand(dart, "pub", "global", "activate", "--source", "path", source)
	result, commandErr := system.Run(ctx, program, arguments, []string{"PUB_CACHE=" + pubCache}, "", "")
	if commandErr != nil {
		return state.FVMSDK{}, &codedError{Code: "MOB_COMMAND_FAILED", Message: "FVM activation failed: " + commandErr.Error(), Remediation: "Review Dart output, then retry mob fvm install."}
	}
	if result.Output != "" && r.json {
		if err := r.emit("log", command, true, map[string]string{"stream": "combined", "output": result.Output}, nil); err != nil {
			return state.FVMSDK{}, err
		}
	}
	if !launcherExists(temporary) {
		return state.FVMSDK{}, &codedError{Code: "MOB_COMMAND_FAILED", Message: "FVM activation completed without creating an executable launcher.", Remediation: "Review Dart output and retry mob fvm install."}
	}
	if err := os.Rename(temporary, destination); err != nil {
		return state.FVMSDK{}, fmt.Errorf("publish Mob-managed FVM: %w", err)
	}
	installed := state.FVMSDK{Version: selected.Version, Path: destination, SHA256: selected.SHA256}
	config.FVM.SDKs = append(config.FVM.SDKs, installed)
	config.FVM.CurrentSDK = installed.Version
	if err := r.store.Save(config); err != nil {
		return state.FVMSDK{}, err
	}
	return installed, nil
}

func (r runtime) ensureFVMRunner(ctx context.Context, noInstall bool, command string) (flutterCommand, error) {
	if launcher, found := system.LookPath("fvm"); found {
		return flutterCommand{Program: launcher, Prefix: []string{"flutter"}}, nil
	}
	config, err := r.store.Load()
	if err != nil {
		return flutterCommand{}, err
	}
	if launcher, found := managedFVMLauncher(config); found {
		return flutterCommand{Program: launcher, Prefix: []string{"flutter"}}, nil
	}
	if noInstall {
		return flutterCommand{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "This Flutter project uses FVM but no FVM launcher is available and automatic installation is disabled.", Remediation: "Run mob fvm install, or rerun without --no-install."}
	}
	if _, err := r.installFVM(ctx, "", command, false); err != nil {
		return flutterCommand{}, err
	}
	config, err = r.store.Load()
	if err != nil {
		return flutterCommand{}, err
	}
	launcher, found := managedFVMLauncher(config)
	if !found {
		return flutterCommand{}, &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Mob installed FVM but its launcher is unavailable."}
	}
	return flutterCommand{Program: launcher, Prefix: []string{"flutter"}}, nil
}

func managedFVMLauncher(config state.Config) (string, bool) {
	for _, sdk := range config.FVM.SDKs {
		if sdk.Version == config.FVM.CurrentSDK && launcherExists(sdk.Path) {
			return fvmLauncherPath(sdk.Path), true
		}
	}
	return "", false
}

func fvmLauncherPath(root string) string {
	name := "fvm"
	if goruntime.GOOS == "windows" {
		name += ".bat"
	}
	return filepath.Join(root, "pub-cache", "bin", name)
}

func launcherExists(root string) bool {
	info, err := os.Stat(fvmLauncherPath(root))
	return err == nil && !info.IsDir()
}

func dartForFlutter(program string) (string, bool) {
	name := "dart"
	if goruntime.GOOS == "windows" {
		name += ".exe"
	}
	if executable, found := system.LookPath(filepath.Join(filepath.Dir(program), name)); found {
		return executable, true
	}
	return system.LookPath(name)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
