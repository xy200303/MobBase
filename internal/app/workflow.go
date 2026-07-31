package app

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/xy200303/MobBase/internal/system"
)

// executeWorkflowCommand preserves the terminal behavior of official mobile
// tools while keeping stdout machine-safe for JSON callers. Event-mode callers
// receive each output line as a structured log event.
func (r runtime) executeWorkflowCommand(ctx context.Context, command, program string, args, environment []string, directory string) (system.CommandResult, error) {
	if r.json && r.eventMode {
		err := system.StreamLines(ctx, program, args, environment, directory, func(line string) error {
			if system.IsWindowsBatchEcho(line) {
				return nil
			}
			return r.emit("log", command, true, map[string]string{"stream": "stdout", "output": line}, nil)
		}, func(line string) error {
			if system.IsWindowsBatchEcho(line) {
				return nil
			}
			return r.emit("log", command, true, map[string]string{"stream": "stderr", "output": line}, nil)
		})
		return system.CommandResult{}, err
	}
	if !r.json {
		external := r.terminal.External(externalToolLabel(program, args))
		var output io.Writer = io.Discard
		if external != nil {
			output = external
		}
		receive := func(line string) error {
			if system.IsWindowsBatchEcho(line) {
				return nil
			}
			_, err := fmt.Fprintln(output, line)
			return err
		}
		err := system.StreamLines(ctx, program, args, environment, directory, receive, receive)
		if external != nil {
			_ = external.Close()
		}
		return system.CommandResult{}, err
	}
	result, err := system.Run(ctx, program, args, environment, directory, "")
	result.Output = system.FilterWindowsBatchEcho(result.Output)
	return result, err
}

func externalToolLabel(program string, args []string) string {
	candidates := append([]string{program}, args...)
	for _, candidate := range candidates {
		name := strings.ToLower(externalExecutableName(candidate))
		switch {
		case strings.HasPrefix(name, "gradlew"), name == "gradle", name == "gradle.exe":
			return "Gradle"
		case strings.HasPrefix(name, "flutter"):
			return "Flutter"
		case name == "xcodebuild":
			return "Xcode"
		case strings.HasPrefix(name, "sdkmanager"):
			return "Android SDK"
		case strings.HasPrefix(name, "avdmanager"):
			return "Android AVD Manager"
		case name == "adb" || name == "adb.exe":
			return "ADB"
		case name == "emulator" || name == "emulator.exe":
			return "Android Emulator"
		}
	}
	name := externalExecutableName(program)
	name = strings.TrimSuffix(name, path.Ext(name))
	if name == "" {
		return "External tool"
	}
	return name
}

func externalExecutableName(value string) string {
	value = strings.Trim(strings.TrimSpace(value), `"'`)
	return path.Base(strings.ReplaceAll(value, `\`, "/"))
}
