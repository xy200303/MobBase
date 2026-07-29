package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestReadLinesDeliversCompleteLines(t *testing.T) {
	var received []string
	err := readLines(strings.NewReader("one\ntwo\n"), func(line string) error {
		received = append(received, line)
		return nil
	})
	if err != nil || len(received) != 2 || received[0] != "one" || received[1] != "two" {
		t.Fatalf("received = %#v, err = %v", received, err)
	}
}

func TestReadLinesStopsOnReceiverError(t *testing.T) {
	want := errors.New("stop")
	err := readLines(strings.NewReader("one\ntwo\n"), func(string) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestStreamLinesReadsChildOutput(t *testing.T) {
	if os.Getenv("MOB_STREAM_LINES_HELPER") == "1" {
		fmt.Fprintln(os.Stdout, "child stdout")
		fmt.Fprintln(os.Stderr, "child stderr")
		return
	}
	var stdout, stderr []string
	err := StreamLines(context.Background(), os.Args[0], []string{"-test.run=^TestStreamLinesReadsChildOutput$"}, []string{"MOB_STREAM_LINES_HELPER=1"}, "", func(line string) error {
		stdout = append(stdout, line)
		return nil
	}, func(line string) error {
		stderr = append(stderr, line)
		return nil
	})
	if err != nil {
		t.Fatalf("stream child: %v", err)
	}
	if !containsLine(stdout, "child stdout") {
		t.Fatalf("stdout = %#v", stdout)
	}
	if !containsLine(stderr, "child stderr") {
		t.Fatalf("stderr = %#v", stderr)
	}
}

func containsLine(lines []string, wanted string) bool {
	for _, line := range lines {
		if line == wanted {
			return true
		}
	}
	return false
}
