package app

import "testing"

func TestParseEnvShell(t *testing.T) {
	shell, show, err := parseEnv([]string{"--shell", "sh"})
	if err != nil || !show || shell != "sh" {
		t.Fatalf("shell=%q show=%t err=%v", shell, show, err)
	}
	shell, show, err = parseEnv([]string{"show", "--shell", "powershell"})
	if err != nil || !show || shell != "powershell" {
		t.Fatalf("shell=%q show=%t err=%v", shell, show, err)
	}
}

func TestInlineHelpPath(t *testing.T) {
	path, requested := inlineHelpPath([]string{"build", "--help"})
	if !requested || len(path) != 1 || path[0] != "build" {
		t.Fatalf("path=%#v requested=%t", path, requested)
	}
}
