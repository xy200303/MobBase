package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xy200303/MobBase/internal/app/ui"
)

func TestTerminalProgressRendersDownloadBarForRedirectedOutput(t *testing.T) {
	var output bytes.Buffer
	terminal := ui.New(nil, &output)
	report := terminal.Download("Downloading Android command-line tools")
	report(512, 1024)
	report(1024, 1024)

	text := output.String()
	if !strings.Contains(text, "50%") || !strings.Contains(text, "100%") {
		t.Fatalf("unexpected download progress: %q", text)
	}
}

func TestTerminalProgressUsesReadablePhaseLabels(t *testing.T) {
	var output bytes.Buffer
	r := runtime{out: &bytes.Buffer{}, err: &output, events: &eventStream{}, terminal: ui.New(nil, &output)}
	if err := r.emit("progress", "mob android sdk install", true, map[string]string{"phase": "bootstrap-command-line-tools"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "Downloading Android command-line tools") {
		t.Fatalf("unexpected terminal event: %q", got)
	}
}

func TestSDKManagerOutputFiltersBatchScriptNoise(t *testing.T) {
	var output bytes.Buffer
	writer := newSDKManagerOutput(&output)
	if _, err := writer.Write([]byte("set APP_HOME=C:\\\\sdk\r\nPreparing \"Install Android SDK Platform 35\".\r\n[=====] 25% Downloading\r\n")); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	text := output.String()
	if strings.Contains(text, "APP_HOME") || !strings.Contains(text, "Preparing") || !strings.Contains(text, "[=====]") {
		t.Fatalf("unexpected filtered sdkmanager output: %q", text)
	}
}

func TestSDKManagerOutputThrottlesRepeatedProgress(t *testing.T) {
	var output bytes.Buffer
	writer := newSDKManagerOutput(&output)
	_, _ = writer.Write([]byte("[==] 20% Downloading\r[===] 21% Downloading\r[====] 25% Downloading\r[====================] 100% Downloading\r"))
	writer.Flush()
	text := output.String()
	if strings.Count(text, "Android SDK:") != 3 || !strings.Contains(text, "20%") || !strings.Contains(text, "25%") || !strings.Contains(text, "100%") {
		t.Fatalf("unexpected throttled output: %q", text)
	}
}

func TestTerminalProgressDoesNotWriteMachineOutput(t *testing.T) {
	var output bytes.Buffer
	r := runtime{json: true, out: &output, err: &bytes.Buffer{}, events: &eventStream{}, terminal: ui.New(&output, &output)}
	if err := r.emit("progress", "mob build", true, map[string]string{"phase": "build"}, nil); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("JSON output contained terminal UI: %q", output.String())
	}
}

func TestDownloadCallbackIsDisabledInJSONMode(t *testing.T) {
	r := runtime{json: true, out: &bytes.Buffer{}, err: &bytes.Buffer{}, events: &eventStream{}, terminal: ui.New(&bytes.Buffer{}, &bytes.Buffer{})}
	if r.download("Downloading test archive") != nil {
		t.Fatal("JSON mode must not render download progress")
	}
}
