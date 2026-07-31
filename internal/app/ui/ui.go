// Package ui renders human-mode terminal output: Mob phases, download progress,
// external-tool output and result tables. JSON output never passes through
// this layer.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/mattn/go-runewidth"
	"github.com/pterm/pterm"
)

const externalViewportLines = 3

// UI renders one command run. ProgressTTY / TableTTY decide between pterm
// rendering and plain output; both are derived from char-device detection so
// redirected output and CI stay machine-friendly.
type UI struct {
	stdout      io.Writer
	stderr      io.Writer
	progressTTY bool
	tableTTY    bool

	mu             sync.Mutex
	active         *download
	activeExternal *ExternalOutput
}

func New(stdout, stderr io.Writer) *UI {
	u := &UI{
		stdout:      stdout,
		stderr:      stderr,
		progressTTY: isCharDevice(stderr),
		tableTTY:    isCharDevice(stdout),
	}
	if !u.progressTTY || !u.tableTTY {
		pterm.DisableStyling()
	}
	return u
}

// newForced is the test hook for exercising the TTY code paths with buffers.
func newForced(stdout, stderr io.Writer, progressTTY, tableTTY bool) *UI {
	return &UI{stdout: stdout, stderr: stderr, progressTTY: progressTTY, tableTTY: tableTTY}
}

func isCharDevice(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// Phase prints one "[mob] command: label" line on stderr, closing any active
// progress bar first so the bar never shares a line with the phase text.
func (u *UI) Phase(command, label string) {
	if u == nil || u.stderr == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.stopActive()
	u.settleExternal()
	line := fmt.Sprintf("[mob] %s: %s", command, label)
	if u.progressTTY {
		line = pterm.FgCyan.Sprint(line)
	}
	fmt.Fprintln(u.stderr, line)
}

// External creates a three-line viewport for one official tool. Interactive
// terminals repaint only the latest three lines in gray; redirected output is
// emitted line by line without ANSI control sequences.
func (u *UI) External(label string) *ExternalOutput {
	if u == nil || u.stderr == nil {
		return nil
	}
	return &ExternalOutput{ui: u, label: strings.TrimSpace(label)}
}

// ExternalOutput accepts arbitrary stdout/stderr chunks from a child process.
// Both carriage returns and newlines are treated as updates because tools such
// as sdkmanager redraw their progress on one terminal line.
type ExternalOutput struct {
	ui      *UI
	label   string
	mu      sync.Mutex
	pending string
	closed  bool

	lines    []string
	rendered int
}

func (e *ExternalOutput) Write(data []byte) (int, error) {
	if e == nil {
		return len(data), nil
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return len(data), nil
	}
	e.pending += strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	lines := make([]string, 0, 2)
	for {
		line, rest, found := strings.Cut(e.pending, "\n")
		if !found {
			break
		}
		e.pending = rest
		lines = append(lines, line)
	}
	e.mu.Unlock()
	for _, line := range lines {
		e.render(line)
	}
	return len(data), nil
}

// Flush renders a final unterminated line without closing the viewport.
func (e *ExternalOutput) Flush() {
	if e == nil {
		return
	}
	e.mu.Lock()
	line := e.pending
	e.pending = ""
	e.mu.Unlock()
	if line != "" {
		e.render(line)
	}
}

// Close leaves the final three lines visible and releases the viewport so the
// next Mob phase or external process starts below it.
func (e *ExternalOutput) Close() error {
	if e == nil {
		return nil
	}
	e.Flush()
	e.mu.Lock()
	e.closed = true
	e.mu.Unlock()
	e.ui.mu.Lock()
	if e.ui.activeExternal == e {
		e.ui.settleExternal()
	}
	e.ui.mu.Unlock()
	return nil
}

func (e *ExternalOutput) render(line string) {
	line = strings.TrimSpace(pterm.RemoveColorFromString(line))
	if line == "" {
		return
	}
	e.ui.mu.Lock()
	defer e.ui.mu.Unlock()
	e.ui.stopActive()
	if e.ui.activeExternal != nil && e.ui.activeExternal != e {
		e.ui.settleExternal()
	}
	e.ui.activeExternal = e
	if !e.ui.progressTTY {
		fmt.Fprintf(e.ui.stderr, "[mob] %s: %s\n", e.label, line)
		return
	}
	e.lines = append(e.lines, line)
	if len(e.lines) > externalViewportLines {
		e.lines = e.lines[len(e.lines)-externalViewportLines:]
	}
	e.repaint()
}

func (e *ExternalOutput) repaint() {
	if e.rendered > 0 {
		fmt.Fprintf(e.ui.stderr, "\x1b[%dA", e.rendered)
	}
	width := pterm.GetTerminalWidth()
	for _, line := range e.lines {
		text := fmt.Sprintf("  %s | %s", e.label, line)
		if width > 4 {
			text = runewidth.Truncate(text, width-1, "…")
		}
		fmt.Fprint(e.ui.stderr, "\r\x1b[2K")
		fmt.Fprintln(e.ui.stderr, pterm.FgGray.Sprint(text))
	}
	e.rendered = len(e.lines)
}

// Download returns a nil-safe progress callback for one download. total may
// be <= 0 when the server did not send a Content-Length. On a TTY with a
// known total this drives a pterm progress bar; every other case degrades to
// throttled single-line logs (every 5%, or every 1 MiB when total is unknown).
func (u *UI) Download(label string) func(downloaded, total int64) {
	if u == nil || u.stderr == nil {
		return nil
	}
	d := &download{ui: u, label: label, lastPercent: -1}
	return d.report
}

type download struct {
	ui          *UI
	label       string
	bar         *pterm.ProgressbarPrinter
	lastPercent int
	nextBytes   int64
	done        bool
}

func (d *download) report(downloaded, total int64) {
	d.ui.mu.Lock()
	defer d.ui.mu.Unlock()
	if d.done {
		return
	}
	percent := -1
	if total > 0 {
		percent = int(downloaded * 100 / total)
		if percent > 100 {
			percent = 100
		}
	}
	if d.ui.progressTTY && percent >= 0 {
		d.reportBar(percent)
		return
	}
	d.reportLine(percent, downloaded, total)
}

func (d *download) reportBar(percent int) {
	if d.bar == nil {
		bar, err := pterm.DefaultProgressbar.
			WithTotal(100).
			WithTitle(d.label).
			WithShowCount(false).
			WithShowElapsedTime(false).
			WithWriter(d.ui.stderr).
			Start()
		if err != nil {
			return
		}
		d.bar = bar
		d.ui.active = d
	}
	if percent == d.lastPercent {
		return
	}
	delta := percent - d.bar.Current
	d.lastPercent = percent
	if delta > 0 {
		d.bar.Add(delta)
	}
	if percent >= 100 {
		d.finish()
	}
}

// finish records the download as complete after the bar auto-stopped at 100%.
func (d *download) finish() {
	d.done = true
	if d.ui.active == d {
		d.ui.active = nil
	}
}

func (d *download) reportLine(percent int, downloaded, total int64) {
	if percent >= 0 && percent != 100 {
		if percent == d.lastPercent || (d.lastPercent >= 0 && percent/5 == d.lastPercent/5) {
			return
		}
	}
	if percent < 0 && downloaded < d.nextBytes {
		return
	}
	d.lastPercent = percent
	if percent < 0 {
		d.nextBytes = downloaded + 1024*1024
	}
	line := progressLine(d.label, downloaded, total, percent)
	if d.ui.progressTTY {
		fmt.Fprintf(d.ui.stderr, "\r%s", line)
		if percent == 100 {
			fmt.Fprintln(d.ui.stderr)
			d.finish()
		}
		return
	}
	fmt.Fprintln(d.ui.stderr, line)
	if percent == 100 {
		d.finish()
	}
}

func progressLine(label string, downloaded, total int64, percent int) string {
	if percent < 0 {
		return fmt.Sprintf("[mob] %s: %s downloaded", label, formatBytes(downloaded))
	}
	return fmt.Sprintf("[mob] %s: %d%% (%s / %s)", label, percent, formatBytes(downloaded), formatBytes(total))
}

// stopActive closes a progress bar that is still below 100%, so the next
// phase line starts on a fresh line.
func (u *UI) stopActive() {
	if u.active == nil {
		return
	}
	if u.active.bar != nil {
		_, _ = u.active.bar.Stop()
	}
	u.active.done = true
	u.active = nil
}

func (u *UI) settleExternal() {
	if u.activeExternal == nil {
		return
	}
	u.activeExternal.rendered = 0
	u.activeExternal = nil
}

// Table renders headers and rows as a pterm table on stdout and returns true.
// When stdout is not a TTY it renders nothing and returns false, so the
// caller can fall back to its legacy tab-separated plain text byte for byte.
func (u *UI) Table(headers []string, rows [][]string) bool {
	if u == nil || u.stdout == nil || !u.tableTTY || len(rows) == 0 {
		return false
	}
	data := make(pterm.TableData, 0, len(rows)+1)
	data = append(data, headers)
	data = append(data, rows...)
	_ = pterm.DefaultTable.WithHasHeader().WithData(data).WithWriter(u.stdout).Render()
	return true
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
