package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	gort "runtime"
	"strings"
	"time"

	"github.com/xy200303/MobBase/internal/system"
)

type androidCreateOptions struct {
	Name, Language, UI string
	MinSDK             int
}

var androidProjectName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

const (
	gradleWrapperVersion  = "8.10.2"
	gradleDistributionURL = "https://downloads.gradle.org/distributions/gradle-8.10.2-bin.zip"
)

func (r runtime) androidCreate(ctx context.Context, args []string) error {
	options, err := parseAndroidCreate(args)
	if err != nil {
		return err
	}
	gradle, err := r.gradleForWrapper(ctx)
	if err != nil {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "A Gradle distribution is required to generate the project Gradle Wrapper: " + err.Error(), Remediation: "Check access to downloads.gradle.org and rerun mob android create."}
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	root := filepath.Join(workingDirectory, options.Name)
	if _, err := os.Stat(root); err == nil {
		return &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Project path already exists: " + root}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := writeAndroidTemplate(root, options); err != nil {
		return err
	}
	program, commandArgs := system.BatchCommand(gradle, "wrapper", "--gradle-version", gradleWrapperVersion)
	result, commandErr := system.Run(ctx, program, commandArgs, nil, root, "")
	if commandErr != nil {
		return &codedError{Code: "MOB_COMMAND_FAILED", Message: "Generate Gradle Wrapper: " + commandErr.Error() + ": " + strings.TrimSpace(result.Output), Remediation: "Project files were created; repair Gradle then run gradle wrapper in the project directory."}
	}
	data := map[string]interface{}{"project": options.Name, "path": root, "language": options.Language, "ui": options.UI, "minSdk": options.MinSDK}
	if r.json {
		return r.result("mob android create", data)
	}
	fmt.Fprintf(r.out, "Created Android project %s.\n", root)
	return nil
}

// gradleForWrapper uses an existing system Gradle when present. Otherwise it
// installs a verified Gradle distribution into Mob's toolchain directory and
// exposes its bin directory through the user's normal command search path.
func (r runtime) gradleForWrapper(ctx context.Context) (string, error) {
	if gradle, found := system.LookPath("gradle"); found {
		return gradle, nil
	}
	destination := filepath.Join(r.home, "toolchains", "gradle", gradleWrapperVersion)
	program := gradleExecutable(destination)
	if regularFile(program) {
		if _, err := system.AddUserPath(filepath.Dir(program)); err != nil {
			return "", fmt.Errorf("make Mob Gradle available globally: %w", err)
		}
		return program, nil
	}
	if _, err := os.Stat(destination); err == nil {
		return "", fmt.Errorf("Mob Gradle runtime is incomplete: %s", destination)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := r.emit("started", "mob android create", true, map[string]string{"phase": "prepare-gradle", "version": gradleWrapperVersion}, nil); err != nil {
		return "", err
	}
	archive, err := r.downloadGradle(ctx)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(destination), "gradle-install-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	if err := system.ExtractZipPrefix(archive, temporary, "gradle-"+gradleWrapperVersion); err != nil {
		return "", err
	}
	if !regularFile(gradleExecutable(temporary)) {
		return "", fmt.Errorf("Gradle archive does not contain its executable")
	}
	if err := os.Rename(temporary, destination); err != nil {
		return "", err
	}
	program = gradleExecutable(destination)
	if _, err := system.AddUserPath(filepath.Dir(program)); err != nil {
		return "", fmt.Errorf("make Mob Gradle available globally: %w", err)
	}
	return program, nil
}

func (r runtime) downloadGradle(ctx context.Context) (string, error) {
	cache := filepath.Join(r.home, "cache", "gradle")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	archive := filepath.Join(cache, "gradle-"+gradleWrapperVersion+"-bin.zip")
	checksumPath := archive + ".sha256"
	checksum, err := gradleChecksum(ctx, checksumPath)
	if err != nil {
		return "", err
	}
	if verifyFileSHA256(archive, checksum) == nil {
		return archive, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, gradleDistributionURL, nil)
	if err != nil {
		return "", err
	}
	response, err := (&http.Client{Timeout: 30 * time.Minute}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("download Gradle: server returned %s", response.Status)
	}
	temporary, err := os.CreateTemp(cache, "gradle-download-*.zip")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	reader := &gradleProgressReader{Reader: response.Body, total: response.ContentLength, report: r.download("Downloading Gradle")}
	reader.notify()
	if _, err := io.Copy(temporary, reader); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := verifyFileSHA256(temporaryPath, checksum); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, archive); err != nil {
		return "", err
	}
	return archive, nil
}

func gradleChecksum(ctx context.Context, cachePath string) (string, error) {
	if data, err := os.ReadFile(cachePath); err == nil {
		if checksum := parseSHA256(string(data)); checksum != "" {
			return checksum, nil
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, gradleDistributionURL+".sha256", nil)
	if err != nil {
		return "", err
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("get Gradle checksum: server returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	if err != nil {
		return "", err
	}
	checksum := parseSHA256(string(data))
	if checksum == "" {
		return "", fmt.Errorf("Gradle checksum response is invalid")
	}
	if err := os.WriteFile(cachePath, []byte(checksum+"\n"), 0o600); err != nil {
		return "", err
	}
	return checksum, nil
}

func gradleExecutable(root string) string {
	name := "gradle"
	if gort.GOOS == "windows" {
		name += ".bat"
	}
	return filepath.Join(root, "bin", name)
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func parseSHA256(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return ""
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return ""
	}
	return strings.ToLower(fields[0])
}

func verifyFileSHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected) {
		return fmt.Errorf("Gradle archive checksum does not match the official checksum")
	}
	return nil
}

type gradleProgressReader struct {
	io.Reader
	downloaded int64
	total      int64
	report     func(downloaded, total int64)
}

func (r *gradleProgressReader) Read(buffer []byte) (int, error) {
	count, err := r.Reader.Read(buffer)
	r.downloaded += int64(count)
	if count > 0 {
		r.notify()
	}
	return count, err
}

func (r *gradleProgressReader) notify() {
	if r.report != nil {
		r.report(r.downloaded, r.total)
	}
}

func parseAndroidCreate(args []string) (androidCreateOptions, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") || !androidProjectName.MatchString(args[0]) {
		return androidCreateOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Android project name is required and must contain only letters, numbers, hyphens, or underscores."}
	}
	options := androidCreateOptions{Name: args[0], Language: "kotlin", UI: "compose", MinSDK: 24}
	for args = args[1:]; len(args) > 0; {
		if len(args) < 2 {
			return androidCreateOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: args[0] + " requires a value."}
		}
		switch args[0] {
		case "--language":
			options.Language = args[1]
		case "--ui":
			options.UI = args[1]
		case "--min-sdk":
			if _, err := fmt.Sscanf(args[1], "%d", &options.MinSDK); err != nil || options.MinSDK < 21 {
				return androidCreateOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--min-sdk must be an integer of at least 21."}
			}
		default:
			return androidCreateOptions{}, invalidCommand("mob android create " + strings.Join(append([]string{options.Name}, args...), " "))
		}
		args = args[2:]
	}
	if options.Language != "kotlin" && options.Language != "java" {
		return androidCreateOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--language must be kotlin or java."}
	}
	if options.UI != "compose" && options.UI != "views" {
		return androidCreateOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "--ui must be compose or views."}
	}
	if options.UI == "compose" && options.Language != "kotlin" {
		return androidCreateOptions{}, &codedError{Code: "MOB_INVALID_ARGUMENT", Message: "Compose projects require --language kotlin.", Remediation: "Use --language kotlin --ui compose, or choose --language java --ui views."}
	}
	return options, nil
}

func writeAndroidTemplate(root string, options androidCreateOptions) error {
	suffix := strings.ToLower(strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(options.Name))
	packageName := "com.example." + suffix
	packagePath := strings.ReplaceAll(packageName, ".", string(filepath.Separator))
	plugin := `id("com.android.application")`
	if options.Language == "kotlin" {
		plugin += `; id("org.jetbrains.kotlin.android")`
	}
	if options.Language == "kotlin" && options.UI == "compose" {
		plugin += `; id("org.jetbrains.kotlin.plugin.compose")`
	}
	rootPlugins := "plugins { id(\"com.android.application\") version \"8.7.3\" apply false; id(\"org.jetbrains.kotlin.android\") version \"2.0.21\" apply false"
	if options.Language == "kotlin" && options.UI == "compose" {
		rootPlugins += "; id(\"org.jetbrains.kotlin.plugin.compose\") version \"2.0.21\" apply false"
	}
	rootPlugins += " }\n"
	files := map[string]string{
		"settings.gradle.kts":              "pluginManagement { repositories { google(); mavenCentral(); gradlePluginPortal() } }\ndependencyResolutionManagement { repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS); repositories { google(); mavenCentral() } }\nrootProject.name = \"" + options.Name + "\"\ninclude(\":app\")\n",
		"build.gradle.kts":                 rootPlugins,
		"app/src/main/AndroidManifest.xml": "<manifest xmlns:android=\"http://schemas.android.com/apk/res/android\"><application android:label=\"" + options.Name + "\"><activity android:name=\".MainActivity\" android:exported=\"true\"><intent-filter><action android:name=\"android.intent.action.MAIN\"/><category android:name=\"android.intent.category.LAUNCHER\"/></intent-filter></activity></application></manifest>\n",
	}
	build := "plugins { " + plugin + " }\nandroid { namespace = \"" + packageName + "\"; compileSdk = 35\n defaultConfig { applicationId = \"" + packageName + "\"; minSdk = " + fmt.Sprint(options.MinSDK) + "; targetSdk = 35; versionCode = 1; versionName = \"1.0.0\" }\n}\n"
	if options.Language == "kotlin" && options.UI == "compose" {
		build += "android { buildFeatures { compose = true } }\ndependencies { implementation(\"androidx.activity:activity-compose:1.9.3\"); implementation(\"androidx.compose.material3:material3:1.3.1\") }\n"
	}
	files["app/build.gradle.kts"] = build
	if options.Language == "kotlin" {
		activity := "package " + packageName + "\n\nclass MainActivity : android.app.Activity()\n"
		if options.UI == "compose" {
			activity = "package " + packageName + "\n\nimport android.os.Bundle\nimport androidx.activity.ComponentActivity\nimport androidx.activity.compose.setContent\nimport androidx.compose.material3.Text\n\nclass MainActivity : ComponentActivity() { override fun onCreate(savedInstanceState: Bundle?) { super.onCreate(savedInstanceState); setContent { Text(\"Hello Mob\") } } }\n"
		}
		files[filepath.Join("app", "src", "main", "java", packagePath, "MainActivity.kt")] = activity
	} else {
		files[filepath.Join("app", "src", "main", "java", packagePath, "MainActivity.java")] = "package " + packageName + ";\npublic class MainActivity extends android.app.Activity { }\n"
	}
	for relative, content := range files {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
