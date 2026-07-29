package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
	"github.com/xy200303/MobBase/internal/system"
)

type releaseOptions struct {
	Platform, Artifact, Output string
	NoInstall, AcceptLicenses  bool
}

func (r runtime) release(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "check" {
		return r.releaseCheck(args[1:])
	}
	o, err := parseRelease(args)
	if err != nil {
		return err
	}
	p, err := project.Detect("")
	if err != nil {
		return err
	}
	if p == nil {
		return &codedError{Code: "MOB_PROJECT_UNRECOGNIZED", Message: "The current directory is not a supported mobile project."}
	}
	platform, err := selectBuildPlatform(p, o.Platform)
	if err != nil {
		return err
	}
	if platform != "android" || (p.Kind != project.KindAndroid && p.Kind != project.KindFlutter) {
		return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: "Android is the only release platform implemented in this Mob release."}
	}
	sdk, requirements, err := r.prepareAndroidSDK(ctx, p.Root, "mob release", false, o.NoInstall, o.AcceptLicenses)
	if err != nil {
		return err
	}
	java, err := r.selectProjectJava(ctx, requirements.JavaVersion, o.NoInstall)
	if err != nil {
		return err
	}
	if err := r.emit("started", "mob release", true, map[string]string{"phase": "release", "platform": "android", "artifact": o.Artifact}, nil); err != nil {
		return err
	}
	if p.Kind == project.KindFlutter {
		if _, err := r.ensureFlutterRunner(ctx, p.Root, o.NoInstall, "mob release"); err != nil {
			return err
		}
	}
	preflight, ready, err := r.releaseCheckData(p)
	if err != nil {
		return err
	}
	if err := r.emit("progress", "mob release", true, map[string]interface{}{"phase": "release-check", "ready": ready, "checks": preflight["checks"]}, nil); err != nil {
		return err
	}
	if !ready {
		return &codedError{Code: "MOB_RELEASE_CONFIGURATION_INVALID", Message: "Release preflight failed.", Remediation: "Run mob release check to review and fix the missing release requirements before building."}
	}
	program, command, err := releaseCommand(p, o.Artifact)
	if err != nil {
		return err
	}
	program, command = system.BatchCommand(program, command...)
	environment := append(androidEnvironment(sdk), javaEnvironment(java)...)
	result, err := r.executeWorkflowCommand(ctx, "mob release", program, command, environment, p.Root)
	if result.Output != "" && !r.json {
		fmt.Fprint(r.out, result.Output)
	}
	if err != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "Android release build failed: " + err.Error(), Remediation: "Review Gradle signing configuration and release build output."}
	}
	artifact, err := findReleaseArtifact(p.Root, o.Artifact)
	if err != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: err.Error(), Remediation: "Ensure the project produces a signed release artifact."}
	}
	if o.Output != "" {
		artifact, err = copyReleaseArtifact(artifact, o.Output)
		if err != nil {
			return err
		}
	}
	info, err := os.Stat(artifact)
	if err != nil {
		return err
	}
	digest, err := sha256File(artifact)
	if err != nil {
		return err
	}
	version, err := project.ReleaseVersion(p)
	if err != nil {
		return err
	}
	data := map[string]interface{}{"platform": "android", "java": java, "artifact": o.Artifact, "path": artifact, "size": info.Size(), "sha256": digest, "version": version}
	if r.json {
		return r.result("mob release", data)
	}
	fmt.Fprintf(r.out, "Android release artifact: %s\n", artifact)
	return nil
}

func (r runtime) releaseCheck(args []string) error {
	if len(args) > 0 && !(len(args) == 2 && args[0] == "--platform" && args[1] == "android") {
		return invalidCommand("mob release check " + strings.Join(args, " "))
	}
	currentProject, err := project.Detect("")
	if err != nil {
		return err
	}
	if currentProject == nil {
		return &codedError{Code: "MOB_PROJECT_UNRECOGNIZED", Message: "The current directory is not a supported mobile project."}
	}
	if _, err := selectBuildPlatform(currentProject, "android"); err != nil {
		return err
	}
	if currentProject.Kind != project.KindAndroid && currentProject.Kind != project.KindFlutter {
		return &codedError{Code: "MOB_PLATFORM_NOT_SUPPORTED", Message: "Android release checks support native Android and Flutter projects only."}
	}
	data, _, err := r.releaseCheckData(currentProject)
	if err != nil {
		return err
	}
	if r.json {
		return r.result("mob release check", data)
	}
	checks := data["checks"].([]check)
	for _, item := range checks {
		fmt.Fprintf(r.out, "%s: %s\n", item.Label, item.Status)
		if item.Fix != "" {
			fmt.Fprintf(r.out, "  %s\n", item.Fix)
		}
	}
	return nil
}

// releaseCheckData is shared by the read-only command and release itself so a
// build cannot bypass checks for the Gradle Wrapper, application identity, or
// release signing configuration.
func (r runtime) releaseCheckData(currentProject *project.Info) (map[string]interface{}, bool, error) {
	config, err := r.store.Load()
	if err != nil {
		return nil, false, err
	}
	sdks, err := android.Discover(config)
	if err != nil {
		return nil, false, err
	}
	requirements, err := project.AndroidRequirementsFor(currentProject.Root)
	if err != nil {
		return nil, false, err
	}
	_, completeSDK := matchingAndroidSDK(sdks, requirements, false)
	checks := []check{{ID: "android-sdk", Label: "Android SDK", Status: status(completeSDK), Required: true, Detail: "No installed Android SDK satisfies the release requirements.", Fix: "Run mob release --accept-licenses to prepare android:managed."}}
	if completeSDK {
		checks[0].Detail = "compatible SDK available"
		checks[0].Fix = ""
	}
	if currentProject.Kind == project.KindFlutter {
		_, runnerErr := flutterRunner(currentProject.Root)
		checks = append(checks, check{ID: "flutter-runner", Label: "Flutter runner", Status: status(runnerErr == nil), Required: true, Detail: detailOr(runnerErr, "available"), Fix: "Install the required flutter or fvm launcher."})
	} else {
		_, _, wrapperErr := buildCommand(currentProject.Root, nil)
		checks = append(checks, check{ID: "gradle-wrapper", Label: "Gradle Wrapper", Status: status(wrapperErr == nil), Required: true, Detail: detailOr(wrapperErr, "available"), Fix: "Restore gradlew/gradlew.bat from the project."})
		_, appErr := project.AndroidApplicationID(currentProject.Root)
		checks = append(checks, check{ID: "application-id", Label: "Application ID", Status: status(appErr == nil), Required: true, Detail: detailOr(appErr, "declared"), Fix: "Declare applicationId in the Android app Gradle module."})
		configured, scanErr := hasReleaseSigningConfig(currentProject.Root)
		checks = append(checks, check{ID: "release-signing", Label: "Release signing", Status: status(configured && scanErr == nil), Required: true, Detail: releaseSigningDetail(configured, scanErr), Fix: "Configure the project's release signingConfig; Mob never creates or stores keys."})
	}
	ready := true
	for _, item := range checks {
		ready = ready && item.Status == "ok"
	}
	return map[string]interface{}{"platform": "android", "ready": ready, "checks": checks}, ready, nil
}

func detailOr(err error, ok string) string {
	if err != nil {
		return err.Error()
	}
	return ok
}
func releaseSigningDetail(configured bool, err error) string {
	if err != nil {
		return err.Error()
	}
	if configured {
		return "release signing configuration found"
	}
	return "no release signing configuration found"
}
func hasReleaseSigningConfig(root string) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e.IsDir() {
			if e.Name() == "build" || e.Name() == ".gradle" {
				return filepath.SkipDir
			}
			return nil
		}
		if e.Name() != "build.gradle" && e.Name() != "build.gradle.kts" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if strings.Contains(text, "signingConfig") && strings.Contains(text, "release") {
			found = true
		}
		return nil
	})
	return found, err
}

func parseRelease(args []string) (releaseOptions, error) {
	o := releaseOptions{Artifact: "aab"}
	for len(args) > 0 {
		if args[0] == "--platform" || args[0] == "--artifact" || args[0] == "--output" {
			if len(args) < 2 {
				return o, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: args[0] + " requires a value."}
			}
			v := args[1]
			if args[0] == "--platform" {
				o.Platform = v
			}
			if args[0] == "--artifact" {
				o.Artifact = v
			}
			if args[0] == "--output" {
				o.Output = v
			}
			args = args[2:]
			continue
		}
		if args[0] == "--no-install" {
			o.NoInstall = true
			args = args[1:]
			continue
		}
		if args[0] == "--accept-licenses" {
			o.AcceptLicenses = true
			args = args[1:]
			continue
		}
		return o, invalidCommand("mob release " + strings.Join(args, " "))
	}
	if o.Artifact != "aab" && o.Artifact != "apk" {
		return o, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--artifact must be aab or apk."}
	}
	return o, nil
}

func releaseCommand(p *project.Info, artifact string) (string, []string, error) {
	if p.Kind == project.KindFlutter {
		runner, err := flutterRunner(p.Root)
		if err != nil {
			return "", nil, err
		}
		target := "appbundle"
		if artifact == "apk" {
			target = "apk"
		}
		return runner.Program, append(runner.Prefix, "build", target, "--release"), nil
	}
	program, args, err := buildCommand(p.Root, nil)
	if err != nil {
		return "", nil, err
	}
	if artifact == "aab" {
		args = []string{"bundleRelease"}
	} else {
		args = []string{"assembleRelease"}
	}
	return program, args, nil
}

func findReleaseArtifact(root, kind string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e.IsDir() {
			if e.Name() == ".gradle" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.Contains(strings.ToLower(path), "release") && strings.EqualFold(filepath.Ext(path), "."+kind) {
			info, err := e.Info()
			if err == nil {
				if prior, statErr := os.Stat(found); found == "" || statErr != nil || info.ModTime().After(prior.ModTime()) {
					found = path
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("no Android release %s artifact was found", kind)
	}
	return filepath.Abs(found)
}
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func copyReleaseArtifact(source, output string) (string, error) {
	info, err := os.Stat(output)
	if err == nil && info.IsDir() {
		output = filepath.Join(output, filepath.Base(source))
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return "", err
	}
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(output)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return filepath.Abs(output)
}
