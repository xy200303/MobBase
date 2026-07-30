package system

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

type CommandResult struct {
	Output string
}

func LookPath(name string) (string, bool) {
	path, err := exec.LookPath(name)
	return path, err == nil
}

func Run(ctx context.Context, program string, args []string, env []string, dir string, input string) (CommandResult, error) {
	return RunWithOutput(ctx, program, args, env, dir, input, nil)
}

// RunWithOutput runs a command while retaining its combined output for callers
// and, when output is provided, forwarding it to an interactive terminal.
// Keeping the captured copy preserves actionable error messages for the CLI.
func RunWithOutput(ctx context.Context, program string, args []string, env []string, dir string, input string, output io.Writer) (CommandResult, error) {
	command := exec.CommandContext(ctx, program, args...)
	command.Env = mergeEnv(env)
	command.Dir = dir
	command.Stdin = strings.NewReader(input)
	var captured bytes.Buffer
	writer := &capturingWriter{capture: &captured, output: output}
	command.Stdout = writer
	command.Stderr = writer
	err := command.Run()
	result := CommandResult{Output: captured.String()}
	if err != nil {
		return result, fmt.Errorf("run %s %s: %w", program, strings.Join(args, " "), err)
	}
	return result, nil
}

// capturingWriter keeps the saved command transcript and optional live output
// in the same order when a child writes on both standard streams.
type capturingWriter struct {
	mu      sync.Mutex
	capture *bytes.Buffer
	output  io.Writer
}

func (w *capturingWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.capture.Write(data); err != nil {
		return 0, err
	}
	if w.output != nil {
		if _, err := w.output.Write(data); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}

func Stream(ctx context.Context, program string, args []string, env []string, dir string, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, program, args...)
	command.Env = mergeEnv(env)
	command.Dir = dir
	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = os.Stdin
	return command.Run()
}

// StreamFiltered streams an official tool command while suppressing cmd.exe's
// echoed .bat implementation lines. Gradle diagnostics and task output remain
// untouched, which keeps interactive failures readable on Windows.
func StreamFiltered(ctx context.Context, program string, args []string, env []string, dir string, stdout, stderr io.Writer) error {
	return StreamLines(ctx, program, args, env, dir, func(line string) error {
		if IsWindowsBatchEcho(line) {
			return nil
		}
		_, err := fmt.Fprintln(stdout, line)
		return err
	}, func(line string) error {
		if IsWindowsBatchEcho(line) {
			return nil
		}
		_, err := fmt.Fprintln(stderr, line)
		return err
	})
}

// FilterWindowsBatchEcho removes the command echo produced when cmd.exe runs
// a .bat file without hiding actual build output.
func FilterWindowsBatchEcho(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if !IsWindowsBatchEcho(line) {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}

// IsWindowsBatchEcho recognizes the Gradle and SDK wrapper commands echoed by
// cmd.exe. A normal Gradle error never has the drive-path prompt prefix.
func IsWindowsBatchEcho(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	command := lower
	hasPrompt := false
	if marker := strings.LastIndex(command, ">"); marker > 1 && strings.Contains(command[:marker], `:\`) {
		command = strings.TrimSpace(command[marker+1:])
		hasPrompt = true
	}
	for _, prefix := range []string{
		"if ", "set dirname=", "set app_base_name=", "set app_home=", "set default_jvm_opts=",
		"set java_exe=", "set classpath=", "for %", "rem ", "@rem ", "\"%java_exe%\"",
	} {
		if strings.HasPrefix(command, prefix) && (hasPrompt || prefix != "if ") {
			return true
		}
	}
	return false
}

// StreamLines runs a child process and delivers complete stdout and stderr
// lines as they arrive. Receivers may return an error to stop the child
// process, which keeps machine protocols from silently losing malformed data.
func StreamLines(ctx context.Context, program string, args []string, env []string, dir string, stdout, stderr func(string) error) error {
	command := exec.CommandContext(ctx, program, args...)
	command.Env = mergeEnv(env)
	command.Dir = dir
	command.Stdin = os.Stdin
	standardOutput, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("prepare stdout for %s: %w", program, err)
	}
	standardError, err := command.StderrPipe()
	if err != nil {
		return fmt.Errorf("prepare stderr for %s: %w", program, err)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start %s %s: %w", program, strings.Join(args, " "), err)
	}

	var receiverErr error
	var receiverMu sync.Mutex
	stop := func(err error) {
		if err == nil {
			return
		}
		receiverMu.Lock()
		if receiverErr == nil {
			receiverErr = err
			_ = command.Process.Kill()
		}
		receiverMu.Unlock()
	}
	done := make(chan struct{}, 2)
	go func() {
		stop(readLines(standardOutput, stdout))
		done <- struct{}{}
	}()
	go func() {
		stop(readLines(standardError, stderr))
		done <- struct{}{}
	}()
	<-done
	<-done
	waitErr := command.Wait()
	receiverMu.Lock()
	defer receiverMu.Unlock()
	if receiverErr != nil {
		return fmt.Errorf("stream %s %s: %w", program, strings.Join(args, " "), receiverErr)
	}
	if waitErr != nil {
		return fmt.Errorf("run %s %s: %w", program, strings.Join(args, " "), waitErr)
	}
	return nil
}

func readLines(reader io.Reader, receive func(string) error) error {
	if receive == nil {
		_, err := io.Copy(io.Discard, reader)
		return err
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := receive(scanner.Text()); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func Start(ctx context.Context, program string, args []string, env []string, dir string) error {
	command := exec.CommandContext(ctx, program, args...)
	command.Env = mergeEnv(env)
	command.Dir = dir
	return command.Start()
}

func BatchCommand(path string, args ...string) (string, []string) {
	if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(path), ".bat") {
		return "cmd.exe", append([]string{"/c", path}, args...)
	}
	return path, args
}

func mergeEnv(extra []string) []string {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			values[strings.ToUpper(parts[0])] = entry
		}
	}
	for _, entry := range extra {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			values[strings.ToUpper(parts[0])] = entry
		}
	}
	result := make([]string, 0, len(values))
	for _, entry := range values {
		result = append(result, entry)
	}
	return result
}
