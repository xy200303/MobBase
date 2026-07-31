package ui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/pterm/pterm"
)

func TestDownloadThrottlesRedirectedOutput(t *testing.T) {
	var output bytes.Buffer
	report := New(nil, &output).Download("Downloading test archive")
	if report == nil {
		t.Fatal("expected a download callback")
	}
	report(512, 1024) // 50%
	report(542, 1024) // 52%, same 5% bucket, suppressed
	report(1024, 1024)

	text := output.String()
	if !strings.Contains(text, "[mob] Downloading test archive: 50% (512 B / 1.0 KiB)") {
		t.Fatalf("missing throttled 50%% line: %q", text)
	}
	if !strings.Contains(text, "[mob] Downloading test archive: 100% (1.0 KiB / 1.0 KiB)") {
		t.Fatalf("missing completion line: %q", text)
	}
	if strings.Contains(text, "52%") {
		t.Fatalf("throttling failed: %q", text)
	}
}

func TestDownloadThrottlesUnknownTotalByMegabyte(t *testing.T) {
	var output bytes.Buffer
	report := New(nil, &output).Download("Downloading test archive")
	report(512*1024, -1)
	report(600*1024, -1) // below the next 1 MiB threshold, suppressed
	report(2*1024*1024, -1)

	text := output.String()
	if !strings.Contains(text, "512.0 KiB downloaded") || !strings.Contains(text, "2.0 MiB downloaded") {
		t.Fatalf("unexpected unknown-total progress: %q", text)
	}
	if strings.Contains(text, "600.0 KiB") {
		t.Fatalf("throttling failed: %q", text)
	}
}

func TestDownloadRendersProgressBarOnTTY(t *testing.T) {
	var output bytes.Buffer
	report := newForced(nil, &output, true, false).Download("Downloading test archive")
	report(512, 1024)
	report(1024, 1024)

	text := output.String()
	if !strings.Contains(text, "Downloading test archive") || !strings.Contains(text, "100%") {
		t.Fatalf("unexpected TTY progress bar output: %q", text)
	}
}

func TestPhaseClosesActiveProgressBar(t *testing.T) {
	var output bytes.Buffer
	terminal := newForced(nil, &output, true, false)
	terminal.Download("Downloading test archive")(512, 1024)
	terminal.Phase("mob build", "Building project")

	text := output.String()
	if !strings.Contains(text, "[mob] mob build: Building project") {
		t.Fatalf("unexpected phase output: %q", text)
	}
}

func TestExternalOutputFallsBackToPlainLines(t *testing.T) {
	var output bytes.Buffer
	external := New(nil, &output).External("Android SDK")
	_, _ = external.Write([]byte("Loading local repository...\rFetch remote repository...\n"))
	_ = external.Close()

	text := output.String()
	if !strings.Contains(text, "[mob] Android SDK: Loading local repository...") || !strings.Contains(text, "[mob] Android SDK: Fetch remote repository...") {
		t.Fatalf("unexpected redirected external output: %q", text)
	}
	if strings.Contains(text, "\x1b[") {
		t.Fatalf("redirected external output contains ANSI controls: %q", text)
	}
}

func TestExternalOutputRepaintsLatestThreeLinesInGray(t *testing.T) {
	pterm.EnableStyling()
	defer pterm.DisableStyling()
	var output bytes.Buffer
	external := newForced(nil, &output, true, false).External("Gradle")
	for _, line := range []string{"first", "second", "third", "fourth"} {
		_, _ = fmt.Fprintln(external, line)
	}
	_ = external.Close()

	text := output.String()
	lastRefresh := strings.LastIndex(text, "\x1b[3A")
	if lastRefresh < 0 {
		t.Fatalf("external viewport did not repaint three lines: %q", text)
	}
	finalFrame := text[lastRefresh:]
	if strings.Contains(finalFrame, "first") || !strings.Contains(finalFrame, "second") || !strings.Contains(finalFrame, "third") || !strings.Contains(finalFrame, "fourth") {
		t.Fatalf("unexpected final external viewport: %q", finalFrame)
	}
	if !strings.Contains(finalFrame, "\x1b[90m") {
		t.Fatalf("external viewport is not gray: %q", finalFrame)
	}
}

func TestPhaseUsesPrimaryColorAndSettlesExternalViewport(t *testing.T) {
	pterm.EnableStyling()
	defer pterm.DisableStyling()
	var output bytes.Buffer
	terminal := newForced(nil, &output, true, false)
	external := terminal.External("Gradle")
	_, _ = fmt.Fprintln(external, "building")
	terminal.Phase("mob build", "Building project")

	text := output.String()
	if !strings.Contains(text, "\x1b[36m[mob] mob build: Building project") {
		t.Fatalf("Mob phase does not use the primary color: %q", text)
	}
}

func TestTableRendersOnlyOnTTY(t *testing.T) {
	var plain bytes.Buffer
	if New(&plain, nil).Table([]string{"NAME", "PATH"}, [][]string{{"a", "b"}}) {
		t.Fatal("non-TTY table must report false")
	}
	if plain.Len() != 0 {
		t.Fatalf("non-TTY table wrote output: %q", plain.String())
	}

	var tty bytes.Buffer
	if !newForced(&tty, nil, false, true).Table([]string{"NAME", "PATH"}, [][]string{{"temurin-17", `C:\jdk`}}) {
		t.Fatal("TTY table must report true")
	}
	text := tty.String()
	if !strings.Contains(text, "NAME") || !strings.Contains(text, "temurin-17") || !strings.Contains(text, `C:\jdk`) {
		t.Fatalf("unexpected TTY table output: %q", text)
	}
}

func TestTableWithNoRowsRendersNothing(t *testing.T) {
	var output bytes.Buffer
	if newForced(&output, nil, false, true).Table([]string{"NAME"}, nil) {
		t.Fatal("empty table must report false")
	}
	if output.Len() != 0 {
		t.Fatalf("empty table wrote output: %q", output.String())
	}
}

func TestNilUIIsSafe(t *testing.T) {
	var terminal *UI
	if callback := terminal.Download("label"); callback != nil {
		t.Fatal("nil UI must return a nil callback")
	}
	terminal.Phase("mob build", "Building project")
	if terminal.Table([]string{"A"}, [][]string{{"b"}}) {
		t.Fatal("nil UI table must report false")
	}
}
