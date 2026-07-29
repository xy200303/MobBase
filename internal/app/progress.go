package app

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/android"
)

type terminalProgress struct {
	output      io.Writer
	interactive bool
	lastPercent int
	nextBytes   int64
	active      bool
}

func newTerminalProgress(output io.Writer) *terminalProgress {
	progress := &terminalProgress{output: output, lastPercent: -1}
	if file, ok := output.(*os.File); ok {
		if info, err := file.Stat(); err == nil {
			progress.interactive = info.Mode()&os.ModeCharDevice != 0
		}
	}
	return progress
}

func (p *terminalProgress) event(kind, command string, data interface{}) {
	if p == nil || p.output == nil || (kind != "started" && kind != "progress") {
		return
	}
	p.finish()
	phase := eventPhase(data)
	if phase == "" {
		phase = kind
	}
	fmt.Fprintf(p.output, "[mob] %s: %s\n", command, phaseLabel(phase))
}

func (p *terminalProgress) download(label string) func(android.DownloadProgress) {
	return func(progress android.DownloadProgress) {
		p.bytes(label, progress.Downloaded, progress.Total)
	}
}

func (p *terminalProgress) bytes(label string, downloaded, total int64) {
	if p == nil || p.output == nil {
		return
	}
	percent := -1
	if total > 0 {
		percent = int(downloaded * 100 / total)
		if percent > 100 {
			percent = 100
		}
	}
	if percent >= 0 && percent != 100 && percent == p.lastPercent {
		return
	}
	if percent >= 0 && percent != 100 && p.lastPercent >= 0 && percent/5 == p.lastPercent/5 {
		return
	}
	if percent < 0 && downloaded < p.nextBytes {
		return
	}
	p.lastPercent = percent
	if percent < 0 {
		p.nextBytes = downloaded + 1024*1024
	}
	line := formatDownloadProgress(label, android.DownloadProgress{Downloaded: downloaded, Total: total}, percent)
	if p.interactive {
		fmt.Fprintf(p.output, "\r%s", line)
		p.active = percent != 100
		if percent == 100 {
			fmt.Fprintln(p.output)
		}
		return
	}
	fmt.Fprintln(p.output, line)
}

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
}

func (w *sdkManagerOutput) writeLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" || w.output == nil {
		return
	}
	isProgress := strings.HasPrefix(line, "[")
	isStatus := strings.Contains(line, "Preparing ") || strings.Contains(line, "Installing ") || strings.Contains(line, "Downloading ") || strings.Contains(line, "Unzipping ") || strings.HasPrefix(line, "Warning:")
	if !isProgress && !isStatus {
		return
	}
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
	fmt.Fprintf(w.output, "[mob] Android SDK: %s\n", line)
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

func (p *terminalProgress) finish() {
	if p != nil && p.active {
		fmt.Fprintln(p.output)
		p.active = false
	}
}

func formatDownloadProgress(label string, progress android.DownloadProgress, percent int) string {
	if percent < 0 {
		return fmt.Sprintf("[mob] %s: %s downloaded", label, formatBytes(progress.Downloaded))
	}
	width := 20
	filled := width * percent / 100
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	return fmt.Sprintf("[mob] %s [%s] %3d%% (%s / %s)", label, bar, percent, formatBytes(progress.Downloaded), formatBytes(progress.Total))
}

func formatBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(size)/(1024*1024))
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
