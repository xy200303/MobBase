package app

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/xy200303/MobBase/internal/platform/android"
	"github.com/xy200303/MobBase/internal/project"
)

type flutterMachineHandler struct {
	runtime runtime
	command string
	device  android.Device
	seen    map[string]bool
}

func shouldUseFlutterMachineDebug(command string, info *project.Info, options runOptions, eventMode bool) bool {
	return command == "mob debug" && info.Kind == project.KindFlutter && eventMode
}

func newFlutterMachineHandler(r runtime, command string, device android.Device) *flutterMachineHandler {
	return &flutterMachineHandler{runtime: r, command: command, device: device, seen: make(map[string]bool)}
}

func (h *flutterMachineHandler) stdout(line string) error {
	var message struct {
		Event  string          `json:"event"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal([]byte(line), &message); err != nil || message.Event != "app.debugPort" {
		return h.log("stdout", line)
	}
	var params struct {
		AppID string      `json:"appId"`
		Port  json.Number `json:"port"`
		WSURI string      `json:"wsUri"`
	}
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return h.log("stdout", line)
	}
	port, err := strconv.Atoi(params.Port.String())
	if err != nil || port <= 0 || strings.TrimSpace(params.WSURI) == "" {
		return h.log("stdout", line)
	}
	key := params.AppID + "\x00" + params.WSURI
	if h.seen[key] {
		return nil
	}
	h.seen[key] = true
	return h.runtime.emit("debugTarget", h.command, true, map[string]interface{}{
		"platform":  "android",
		"transport": "dart-vm-service",
		"device":    h.device,
		"appId":     params.AppID,
		"port":      port,
		"wsUri":     params.WSURI,
	}, nil)
}

func (h *flutterMachineHandler) stderr(line string) error {
	return h.log("stderr", line)
}

func (h *flutterMachineHandler) log(stream, output string) error {
	return h.runtime.emit("log", h.command, true, map[string]string{"stream": stream, "output": output}, nil)
}
