package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
	"github.com/xy200303/MobBase/internal/system"
)

type buildOptions struct {
	Platform       string
	Command        []string
	Force          bool
	NoInstall      bool
	AcceptLicenses bool
}

func (r runtime) build(ctx context.Context, args []string) error {
	options, err := parseBuild(args)
	if err != nil {
		return err
	}
	currentProject, err := project.Detect("")
	if err != nil {
		return err
	}
	if options.Force && (options.Platform != "android" || len(options.Command) == 0) {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--force requires --platform android and an official command after --.", Remediation: "Use mob build --force --platform android -- <gradle-wrapper> <task>."}
	}
	if currentProject == nil && options.Force {
		root, rootErr := os.Getwd()
		if rootErr != nil {
			return fmt.Errorf("resolve project directory: %w", rootErr)
		}
		currentProject = &project.Info{Root: root, Kind: project.KindAndroid, Targets: []string{"android"}}
	}
	if currentProject == nil {
		return &codedError{Code: "MOB_PROJECT_UNRECOGNIZED", Message: "The current directory is not a supported mobile project.", Remediation: "Run mob status, or pass a supported project directory to your terminal."}
	}
	platform, err := selectBuildPlatformWithForce(currentProject, options.Platform, options.Force)
	if err != nil {
		return err
	}
	if platform == "ios" {
		return r.buildIOS(ctx, currentProject, options)
	}
	if platform != "android" {
		return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: "Android is the only build platform implemented in this Mob release.", Remediation: "Choose --platform android for a project that declares an Android target."}
	}
	if currentProject.Kind != project.KindAndroid && currentProject.Kind != project.KindFlutter && currentProject.Kind != project.KindKotlinMultiplatform && !(options.Force && len(options.Command) > 0) {
		return &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: "The " + string(currentProject.Kind) + " build adapter is not available yet.", Remediation: "Use the project's official runner for now, or build a native Android Gradle project with mob build."}
	}
	sdk, requirements, err := r.prepareAndroidSDK(ctx, currentProject.Root, "mob build", false, options.NoInstall, options.AcceptLicenses)
	if err != nil {
		return err
	}
	java, err := r.selectProjectJava(ctx, requirements.JavaVersion, options.NoInstall)
	if err != nil {
		return err
	}
	if err := r.emit("started", "mob build", true, map[string]interface{}{"phase": "build", "platform": "android", "project": currentProject.Root, "sdk": sdk.Name}, nil); err != nil {
		return err
	}
	if currentProject.Kind == project.KindFlutter && len(options.Command) == 0 {
		if _, err := r.ensureFlutterRunner(ctx, currentProject.Root, options.NoInstall, "mob build"); err != nil {
			return err
		}
	}
	program, commandArgs, err := buildProjectCommand(currentProject, options.Command)
	if err != nil {
		return err
	}
	program, commandArgs = system.BatchCommand(program, commandArgs...)
	environment := append(androidEnvironment(sdk), javaEnvironment(java)...)
	result, commandErr := r.executeWorkflowCommand(ctx, "mob build", program, commandArgs, environment, currentProject.Root)
	if result.Output != "" {
		if r.json {
			if err := r.emit("log", "mob build", true, map[string]string{"stream": "combined", "output": result.Output}, nil); err != nil {
				return err
			}
		} else {
			fmt.Fprint(r.out, result.Output)
		}
	}
	if commandErr != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "Android build failed: " + commandErr.Error(), Remediation: "Review the Gradle output, project requirements, and selected Android SDK."}
	}
	data := map[string]interface{}{"platform": "android", "project": currentProject.Root, "sdk": sdk.Name, "java": java, "command": append([]string{program}, commandArgs...)}
	if r.json {
		return r.result("mob build", data)
	}
	fmt.Fprintln(r.out, "Android build completed.")
	return nil
}

func (r runtime) buildIOS(ctx context.Context, currentProject *project.Info, options buildOptions) error {
	if currentProject.Kind != project.KindIOS {
		return &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: "The " + string(currentProject.Kind) + " iOS build adapter is not available yet.", Remediation: "Use the framework's official iOS runner for now, or build a native Xcode project with mob build --platform ios."}
	}
	toolchain, err := r.iosToolchain(ctx, "mob build --platform ios")
	if err != nil {
		return err
	}
	program, commandArgs, err := iosBuildCommand(currentProject.Root, options.Command)
	if err != nil {
		return &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: "Prepare native iOS build: " + err.Error(), Remediation: "Ensure the project has one valid .xcodeproj, or pass an explicit official xcodebuild command after --."}
	}
	if err := r.emit("started", "mob build", true, map[string]interface{}{"phase": "build", "platform": "ios", "project": currentProject.Root, "developerDir": toolchain.DeveloperDir}, nil); err != nil {
		return err
	}
	result, commandErr := r.executeWorkflowCommand(ctx, "mob build", program, commandArgs, []string{"DEVELOPER_DIR=" + toolchain.DeveloperDir}, currentProject.Root)
	if result.Output != "" && !r.json {
		fmt.Fprint(r.out, result.Output)
	}
	if commandErr != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "iOS build failed: " + commandErr.Error(), Remediation: "Review the xcodebuild output, selected Xcode, scheme, signing configuration, and project build settings."}
	}
	data := map[string]interface{}{"platform": "ios", "project": currentProject.Root, "developerDir": toolchain.DeveloperDir, "xcode": toolchain.Version, "buildVersion": toolchain.BuildVersion, "command": append([]string{program}, commandArgs...)}
	if r.json {
		return r.result("mob build", data)
	}
	fmt.Fprintln(r.out, "iOS build completed.")
	return nil
}

func buildProjectCommand(info *project.Info, forwarded []string) (string, []string, error) {
	if info.Kind == project.KindFlutter {
		return flutterBuildCommand(info.Root, forwarded)
	}
	return buildCommand(info.Root, forwarded)
}

func iosBuildCommand(root string, forwarded []string) (string, []string, error) {
	if len(forwarded) > 0 {
		return forwarded[0], forwarded[1:], nil
	}
	projectPath, err := project.IOSProjectPath(root)
	if err != nil {
		return "", nil, err
	}
	return "xcodebuild", []string{"-project", projectPath, "-configuration", "Debug", "build"}, nil
}

func parseBuild(args []string) (buildOptions, error) {
	options := buildOptions{}
	for len(args) > 0 {
		if args[0] == "--" {
			if len(args) == 1 {
				return buildOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "-- must be followed by an official build command."}
			}
			options.Command = append([]string(nil), args[1:]...)
			return options, nil
		}
		switch args[0] {
		case "--platform":
			if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
				return buildOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--platform requires a platform ID."}
			}
			options.Platform = args[1]
			args = args[2:]
		case "--force":
			options.Force = true
			args = args[1:]
		case "--no-install":
			options.NoInstall = true
			args = args[1:]
		case "--accept-licenses":
			options.AcceptLicenses = true
			args = args[1:]
		default:
			return buildOptions{}, invalidCommand("mob build " + strings.Join(args, " "))
		}
	}
	return options, nil
}

func selectBuildPlatform(info *project.Info, requested string) (string, error) {
	return selectBuildPlatformWithForce(info, requested, false)
}

func selectBuildPlatformWithForce(info *project.Info, requested string, force bool) (string, error) {
	if requested != "" {
		for _, target := range info.Targets {
			if target == requested {
				return requested, nil
			}
		}
		if force {
			return requested, nil
		}
		article := "a"
		if requested == "android" || requested == "ios" {
			article = "an"
		}
		return "", &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: "The current project does not declare " + article + " " + requested + " target.", Remediation: "Run mob status and choose a declared platform, or use --force --platform android -- <official-command> to run a verified Gradle command."}
	}
	if len(info.Targets) == 1 {
		return info.Targets[0], nil
	}
	return "", &codedError{Code: "MOB_PLATFORM_REQUIRED", Message: "The current project declares multiple build targets.", Remediation: "Pass --platform <id>, for example --platform android."}
}

func selectAndroidBuildSDK(sdks []android.SDK) (android.SDK, bool) {
	for _, sdk := range sdks {
		if sdk.Current {
			return sdk, true
		}
	}
	if len(sdks) > 0 {
		return sdks[0], true
	}
	return android.SDK{}, false
}

func buildCommand(root string, forwarded []string) (string, []string, error) {
	if len(forwarded) > 0 {
		return forwarded[0], forwarded[1:], nil
	}
	wrapper := "gradlew"
	if goruntime.GOOS == "windows" {
		wrapper = "gradlew.bat"
	}
	path := filepath.Join(root, wrapper)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", nil, &codedError{Code: "MOB_RUNNER_UNAVAILABLE", Message: "Gradle Wrapper was not found in the Android project.", Remediation: "Restore gradlew/gradlew.bat from the project, or pass an explicit official build command after --."}
	}
	return path, []string{"assembleDebug"}, nil
}

func androidEnvironment(sdk android.SDK) []string {
	return []string{"ANDROID_SDK_ROOT=" + sdk.Path, "ANDROID_HOME=" + sdk.Path}
}
