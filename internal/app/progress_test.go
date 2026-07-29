package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xy200303/MobBase/internal/platform/android"
)

func TestTerminalProgressRendersDownloadBarForRedirectedOutput(t *testing.T) {
	var output bytes.Buffer
	progress := newTerminalProgress(&output)
	progress.download("Downloading Android command-line tools")(android.DownloadProgress{Downloaded: 512, Total: 1024})
	progress.download("Downloading Android command-line tools")(android.DownloadProgress{Downloaded: 1024, Total: 1024})

	text := output.String()
	if !strings.Contains(text, "[##########----------]  50%") || !strings.Contains(text, "[####################] 100%") {
		t.Fatalf("unexpected download progress: %q", text)
	}
}

func TestTerminalProgressUsesReadablePhaseLabels(t *testing.T) {
	var output bytes.Buffer
	progress := newTerminalProgress(&output)
	progress.event("progress", "mob android sdk install", map[string]string{"phase": "bootstrap-command-line-tools"})
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
	r := runtime{json: true, out: &output, err: &bytes.Buffer{}, events: &eventStream{}, terminal: newTerminalProgress(&output)}
	if err := r.emit("progress", "mob build", true, map[string]string{"phase": "build"}, nil); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("JSON output contained terminal UI: %q", output.String())
	}
}
