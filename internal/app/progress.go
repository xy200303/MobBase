package app

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xy200303/MobBase/internal/system"
)

type sdkManagerOutput struct {
	output       io.Writer
	pending      string
	lastProgress int
	lastLine     string
}

func newSDKManagerOutput(output io.Writer) *sdkManagerOutput {
	return &sdkManagerOutput{output: output, lastProgress: -1}
}

func (w *sdkManagerOutput) Write(data []byte) (int, error) {
	w.pending += strings.ReplaceAll(string(data), "\r", "\n")
	for {
		line, rest, found := strings.Cut(w.pending, "\n")
		if !found {
			break
		}
		w.pending = rest
		w.writeLine(line)
	}
	return len(data), nil
}

func (w *sdkManagerOutput) Flush() {
	if w == nil {
		return
	}
	w.writeLine(w.pending)
	w.pending = ""
	if flusher, ok := w.output.(interface{ Flush() }); ok {
		flusher.Flush()
	}
	if closer, ok := w.output.(io.Closer); ok {
		_ = closer.Close()
	}
}

func (w *sdkManagerOutput) writeLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" || w.output == nil {
		return
	}
	if system.IsWindowsBatchEcho(line) {
		return
	}
	isProgress := strings.HasPrefix(line, "[")
	if isProgress {
		if percent, found := sdkManagerProgressPercent(line); found {
			if percent != 100 && w.lastProgress >= 0 && percent/5 == w.lastProgress/5 {
				return
			}
			w.lastProgress = percent
		} else if line == w.lastLine {
			return
		}
	} else {
		w.lastProgress = -1
	}
	if line == w.lastLine {
		return
	}
	w.lastLine = line
	fmt.Fprintln(w.output, line)
}

func sdkManagerProgressPercent(line string) (int, bool) {
	percentIndex := strings.LastIndex(line, "%")
	if percentIndex < 1 {
		return 0, false
	}
	start := percentIndex - 1
	for start >= 0 && line[start] >= '0' && line[start] <= '9' {
		start--
	}
	value, err := strconv.Atoi(line[start+1 : percentIndex])
	if err != nil || value < 0 || value > 100 {
		return 0, false
	}
	return value, true
}

func eventPhase(data interface{}) string {
	if values, ok := data.(map[string]string); ok {
		return values["phase"]
	}
	if values, ok := data.(map[string]interface{}); ok {
		if phase, ok := values["phase"].(string); ok {
			return phase
		}
	}
	return ""
}

func phaseLabel(phase string) string {
	return strings.NewReplacer(
		"bootstrap-command-line-tools", "Downloading Android command-line tools",
		"prepare-toolchain", "Preparing Android toolchain",
		"prepare-emulator", "Preparing Android emulator",
		"download-flutter", "Downloading Flutter SDK",
		"bootstrap-scrcpy", "Preparing Android device preview",
		"create-device", "Creating Android virtual device",
		"start-device", "Starting Android virtual device",
		"wait-device", "Waiting for Android device",
		"install", "Installing components",
		"build", "Building project",
		"run", "Running project",
		"debug", "Preparing debugger",
	).Replace(phase)
}
