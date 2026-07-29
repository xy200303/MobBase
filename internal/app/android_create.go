package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xy200303/MobBase/internal/system"
)

type androidCreateOptions struct {
	Name, Language, UI string
	MinSDK             int
}

var androidProjectName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

func (r runtime) androidCreate(ctx context.Context, args []string) error {
	options, err := parseAndroidCreate(args)
	if err != nil {
		return err
	}
	gradle, found := system.LookPath("gradle")
	if !found {
		return &codedError{Code: "MOB_TOOLCHAIN_MISSING", Message: "Gradle was not found on PATH, so a Gradle Wrapper cannot be created.", Remediation: "Install Gradle through its official distribution, then rerun mob android create."}
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
	program, commandArgs := system.BatchCommand(gradle, "wrapper", "--gradle-version", "8.10.2")
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
