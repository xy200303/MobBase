package android

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xy200303/MobBase/internal/system"
)

// Device is the Android projection of Mob's cross-platform device model.
type Device struct {
	ID       string `json:"id"`
	Platform string `json:"platform"`
	NativeID string `json:"nativeId"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	State    string `json:"state"`
}

// Emulator is an Android Virtual Device definition reported by the official
// emulator binary. AVD names are distinct from ADB runtime device IDs.
type Emulator struct {
	Name string `json:"name"`
}

var avdNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var pairingCodePattern = regexp.MustCompile(`^\d{6}$`)

// CreateEmulator creates an AVD from an already installed SDK system image.
// It never overwrites an existing AVD and never installs or removes SDK files.
func CreateEmulator(ctx context.Context, sdk SDK, name, image string) error {
	if !avdNamePattern.MatchString(name) {
		return fmt.Errorf("invalid Android virtual device name %q", name)
	}
	if !contains(sdk.Components.SystemImages, image) {
		return fmt.Errorf("Android system image %q is not installed in SDK %s", image, sdk.Name)
	}
	emulators, err := ListEmulators(ctx, []SDK{sdk})
	if err != nil {
		return err
	}
	for _, emulator := range emulators {
		if emulator.Name == name {
			return fmt.Errorf("Android virtual device %q already exists", name)
		}
	}
	manager, found := AVDManager(sdk.Path)
	if !found {
		return fmt.Errorf("Android AVD Manager was not found in %s", sdk.Path)
	}
	program, args := system.BatchCommand(manager, "create", "avd", "--name", name, "--package", image)
	result, err := system.Run(ctx, program, args, nil, "", "no\n")
	if err != nil {
		return fmt.Errorf("create Android virtual device %s: %w: %s", name, err, strings.TrimSpace(result.Output))
	}
	return nil
}

func DefaultSystemImage(sdk SDK) (string, bool) {
	if len(sdk.Components.SystemImages) == 0 {
		return "", false
	}
	return sdk.Components.SystemImages[0], true
}

// SystemImageForAPI returns an installed system image for the requested API.
func SystemImageForAPI(sdk SDK, api int) (string, bool) {
	prefix := fmt.Sprintf("system-images;android-%d;", api)
	for _, image := range sdk.Components.SystemImages {
		if strings.HasPrefix(image, prefix) {
			return image, true
		}
	}
	return "", false
}

func DefaultEmulatorName(image string) string {
	parts := strings.Split(image, ";")
	if len(parts) >= 2 {
		return "mob-" + strings.Replace(parts[1], "android-", "android-api-", 1)
	}
	return "mob-android"
}

func ListEmulators(ctx context.Context, sdks []SDK) ([]Emulator, error) {
	emulator, ok := findEmulator(sdks)
	if !ok {
		return nil, fmt.Errorf("Android Emulator was not found")
	}
	result, err := system.Run(ctx, emulator, []string{"-list-avds"}, nil, "", "")
	if err != nil {
		return nil, fmt.Errorf("list Android emulators: %w: %s", err, strings.TrimSpace(result.Output))
	}
	return ParseEmulators(result.Output), nil
}

func ParseEmulators(output string) []Emulator {
	seen := make(map[string]bool)
	emulators := make([]Emulator, 0)
	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		emulators = append(emulators, Emulator{Name: name})
	}
	sort.Slice(emulators, func(i, j int) bool { return emulators[i].Name < emulators[j].Name })
	return emulators
}

// StartEmulator only launches an existing AVD. It does not create, replace or
// modify an AVD definition.
func StartEmulator(ctx context.Context, sdks []SDK, avd string) error {
	return StartEmulatorWithOptions(ctx, sdks, avd, false)
}

// StartEmulatorWithOptions launches an existing AVD through the official
// Emulator binary. Headless mode suppresses its native window for CI.
func StartEmulatorWithOptions(ctx context.Context, sdks []SDK, avd string, headless bool) error {
	emulator, ok := findEmulator(sdks)
	if !ok {
		return fmt.Errorf("Android Emulator was not found")
	}
	emulators, err := ListEmulators(ctx, sdks)
	if err != nil {
		return err
	}
	for _, item := range emulators {
		if item.Name == avd {
			arguments := []string{"-avd", avd}
			if headless {
				arguments = append(arguments, "-no-window")
			}
			return system.Start(ctx, emulator, arguments, nil, "")
		}
	}
	return fmt.Errorf("Android virtual device %q was not found", avd)
}

func StopEmulator(ctx context.Context, sdks []SDK, id string) error {
	nativeID := strings.TrimPrefix(id, "android:")
	if !strings.HasPrefix(nativeID, "emulator-") {
		return fmt.Errorf("%q is not an Android emulator device ID", id)
	}
	adb, ok := findADB(sdks)
	if !ok {
		return fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "emu", "kill"}, nil, "", "")
	if err != nil {
		return fmt.Errorf("stop Android emulator %s: %w: %s", nativeID, err, strings.TrimSpace(result.Output))
	}
	return nil
}

// WaitForReadyEmulator waits until ADB reports an emulator as ready. The
// caller supplies a timeout through ctx so this function never waits forever.
func WaitForReadyEmulator(ctx context.Context, sdks []SDK) (Device, error) {
	return WaitForNewReadyEmulator(ctx, sdks, nil)
}

// WaitForNewReadyEmulator waits for an emulator that was not present in known.
// It lets callers distinguish a newly launched AVD from other ready emulators
// that may already be connected to ADB.
func WaitForNewReadyEmulator(ctx context.Context, sdks []SDK, known []Device) (Device, error) {
	existing := readyEmulatorIDs(known)
	for {
		devices, err := ListDevices(ctx, sdks)
		if err != nil {
			return Device{}, err
		}
		if device, found := readyEmulatorNotIn(devices, existing); found {
			return device, nil
		}
		select {
		case <-ctx.Done():
			return Device{}, fmt.Errorf("Android emulator did not become ready: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func readyEmulatorNotIn(devices []Device, excluded map[string]struct{}) (Device, bool) {
	for _, device := range devices {
		if device.Kind == "emulator" && device.State == "ready" {
			if _, exists := excluded[device.ID]; !exists {
				return device, true
			}
		}
	}
	return Device{}, false
}

func readyEmulatorIDs(devices []Device) map[string]struct{} {
	ids := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		if device.Kind == "emulator" && device.State == "ready" {
			ids[device.ID] = struct{}{}
		}
	}
	return ids
}

func ListDevices(ctx context.Context, sdks []SDK) ([]Device, error) {
	adb, ok := findADB(sdks)
	if !ok {
		return nil, fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"devices", "-l"}, nil, "", "")
	if err != nil {
		return nil, fmt.Errorf("list Android devices: %w: %s", err, strings.TrimSpace(result.Output))
	}
	return ParseDevices(result.Output), nil
}

func ConnectDevice(ctx context.Context, sdks []SDK, address string) (string, error) {
	if _, _, err := net.SplitHostPort(address); err != nil {
		return "", fmt.Errorf("invalid Android device address %q: use host:port", address)
	}
	adb, ok := findADB(sdks)
	if !ok {
		return "", fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"connect", address}, nil, "", "")
	if err != nil {
		return "", fmt.Errorf("connect Android device: %w: %s", err, strings.TrimSpace(result.Output))
	}
	return strings.TrimSpace(result.Output), nil
}

// PairDevice completes Android Wireless Debugging pairing through the official
// ADB protocol. The pairing address is deliberately separate from the later
// device connection address shown by Android.
func PairDevice(ctx context.Context, sdks []SDK, address, code string) (string, error) {
	if _, _, err := net.SplitHostPort(address); err != nil {
		return "", fmt.Errorf("invalid Android pairing address %q: use host:port", address)
	}
	if !pairingCodePattern.MatchString(code) {
		return "", fmt.Errorf("Android pairing code must contain exactly 6 digits")
	}
	adb, ok := findADB(sdks)
	if !ok {
		return "", fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"pair", address, code}, nil, "", "")
	if err != nil {
		return "", fmt.Errorf("pair Android device: %w: %s", err, strings.TrimSpace(result.Output))
	}
	return strings.TrimSpace(result.Output), nil
}

func ScreenshotDevice(ctx context.Context, sdks []SDK, nativeID, output string) error {
	adb, ok := findADB(sdks)
	if !ok {
		return fmt.Errorf("Android Debug Bridge was not found")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, adb, "-s", nativeID, "exec-out", "screencap", "-p")
	command.Stdout = file
	var stderr strings.Builder
	command.Stderr = &stderr
	err = command.Run()
	closeErr := file.Close()
	if err != nil {
		return fmt.Errorf("capture Android screenshot: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

// UIHierarchyXML returns the current UI Automator hierarchy without writing a
// host-visible temporary file on the Android device.
func UIHierarchyXML(ctx context.Context, sdks []SDK, nativeID string) ([]byte, error) {
	adb, ok := findADB(sdks)
	if !ok {
		return nil, fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "exec-out", "sh", "-c", "uiautomator dump /sdcard/mob-ui.xml >/dev/null && { cat /sdcard/mob-ui.xml; rm -f /sdcard/mob-ui.xml; }"}, nil, "", "")
	if err != nil {
		return nil, fmt.Errorf("dump Android UI hierarchy: %w: %s", err, strings.TrimSpace(result.Output))
	}
	return []byte(result.Output), nil
}

func WaitForBoot(ctx context.Context, sdks []SDK, nativeID string) error {
	adb, ok := findADB(sdks)
	if !ok {
		return fmt.Errorf("Android Debug Bridge was not found")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		result, err := system.Run(ctx, adb, []string{"-s", nativeID, "shell", "getprop", "sys.boot_completed"}, nil, "", "")
		if err == nil && strings.TrimSpace(result.Output) == "1" {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Android boot: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func WaitForIdle(ctx context.Context, sdks []SDK, nativeID string) error {
	adb, ok := findADB(sdks)
	if !ok {
		return fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "shell", "uiautomator", "waitforidle"}, nil, "", "")
	if err != nil {
		return fmt.Errorf("wait for Android UI idle: %w: %s", err, strings.TrimSpace(result.Output))
	}
	return nil
}

func Input(ctx context.Context, sdks []SDK, nativeID string, args []string) error {
	adb, ok := findADB(sdks)
	if !ok {
		return fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, append([]string{"-s", nativeID, "shell", "input"}, args...), nil, "", "")
	if err != nil {
		return fmt.Errorf("send Android input: %w: %s", err, strings.TrimSpace(result.Output))
	}
	return nil
}

func RecordDevice(ctx context.Context, sdks []SDK, nativeID, output string, seconds int) error {
	adb, ok := findADB(sdks)
	if !ok {
		return fmt.Errorf("Android Debug Bridge was not found")
	}
	remote := "/sdcard/mob-record.mp4"
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "shell", "screenrecord", "--time-limit", strconv.Itoa(seconds), remote}, nil, "", "")
	if err != nil {
		return fmt.Errorf("record Android screen: %w: %s", err, strings.TrimSpace(result.Output))
	}
	defer system.Run(context.Background(), adb, []string{"-s", nativeID, "shell", "rm", "-f", remote}, nil, "", "")
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	result, err = system.Run(ctx, adb, []string{"-s", nativeID, "pull", remote, output}, nil, "", "")
	if err != nil {
		return fmt.Errorf("pull Android recording: %w: %s", err, strings.TrimSpace(result.Output))
	}
	return nil
}

func LaunchPackage(ctx context.Context, sdks []SDK, nativeID, packageName string) error {
	adb, ok := findADB(sdks)
	if !ok {
		return fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "shell", "monkey", "-p", packageName, "1"}, nil, "", "")
	if err != nil {
		return fmt.Errorf("launch Android package %s: %w: %s", packageName, err, strings.TrimSpace(result.Output))
	}
	return nil
}

// WakeDevice brings a connected Android device out of sleep. For emulators
// this makes the already-running official Emulator window immediately usable.
func WakeDevice(ctx context.Context, sdks []SDK, nativeID string) error {
	adb, ok := findADB(sdks)
	if !ok {
		return fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "shell", "input", "keyevent", "KEYCODE_WAKEUP"}, nil, "", "")
	if err != nil {
		return fmt.Errorf("wake Android device %s: %w: %s", nativeID, err, strings.TrimSpace(result.Output))
	}
	return nil
}

// SetDebugApp makes Android wait for a JDWP debugger when the package starts.
func SetDebugApp(ctx context.Context, sdks []SDK, nativeID, packageName string) error {
	adb, ok := findADB(sdks)
	if !ok {
		return fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "shell", "am", "set-debug-app", "-w", packageName}, nil, "", "")
	if err != nil {
		return fmt.Errorf("set Android debug app %s: %w: %s", packageName, err, strings.TrimSpace(result.Output))
	}
	return nil
}

// WaitForJDWPProcess returns the named package's debuggable process exposed
// by ADB. The supplied context controls how long a newly launched app may
// take to appear.
func WaitForJDWPProcess(ctx context.Context, sdks []SDK, nativeID, packageName string) (int, error) {
	adb, ok := findADB(sdks)
	if !ok {
		return 0, fmt.Errorf("Android Debug Bridge was not found")
	}
	for {
		packageResult, packageErr := system.Run(ctx, adb, []string{"-s", nativeID, "shell", "pidof", packageName}, nil, "", "")
		if packageErr == nil {
			for _, value := range strings.Fields(packageResult.Output) {
				pid, parseErr := strconv.Atoi(value)
				if parseErr != nil || pid <= 0 {
					continue
				}
				result, err := system.Run(ctx, adb, []string{"-s", nativeID, "jdwp"}, nil, "", "")
				if err == nil && containsPID(result.Output, pid) {
					return pid, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("Android JDWP process did not become available: %w", ctx.Err())
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// ForwardJDWP exposes one debuggable Android process on a loopback TCP port
// using the official ADB transport. The returned port is intentionally chosen
// by the OS so multiple projects and devices can debug concurrently.
func ForwardJDWP(ctx context.Context, sdks []SDK, nativeID string, pid int) (int, error) {
	if pid <= 0 {
		return 0, fmt.Errorf("Android JDWP PID must be positive")
	}
	adb, ok := findADB(sdks)
	if !ok {
		return 0, fmt.Errorf("Android Debug Bridge was not found")
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		return 0, fmt.Errorf("reserve local JDWP port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, fmt.Errorf("release local JDWP port: %w", err)
	}
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "forward", "tcp:" + strconv.Itoa(port), "jdwp:" + strconv.Itoa(pid)}, nil, "", "")
	if err != nil {
		return 0, fmt.Errorf("forward Android JDWP process %d: %w: %s", pid, err, strings.TrimSpace(result.Output))
	}
	return port, nil
}

// RemoveJDWPForward removes a loopback ADB forward previously created for a
// JDWP debugger. It changes only the active ADB daemon's forwarding table.
func RemoveJDWPForward(ctx context.Context, sdks []SDK, nativeID string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("Android JDWP port must be between 1 and 65535")
	}
	adb, ok := findADB(sdks)
	if !ok {
		return fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "forward", "--remove", "tcp:" + strconv.Itoa(port)}, nil, "", "")
	if err != nil {
		return fmt.Errorf("remove Android JDWP forward %d: %w: %s", port, err, strings.TrimSpace(result.Output))
	}
	return nil
}

// PackagePID returns the first running PID for an application package.
func PackagePID(ctx context.Context, sdks []SDK, nativeID, packageName string) (int, error) {
	adb, ok := findADB(sdks)
	if !ok {
		return 0, fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "shell", "pidof", packageName}, nil, "", "")
	if err != nil {
		return 0, fmt.Errorf("find Android package %s: %w: %s", packageName, err, strings.TrimSpace(result.Output))
	}
	for _, value := range strings.Fields(result.Output) {
		pid, parseErr := strconv.Atoi(value)
		if parseErr == nil && pid > 0 {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("Android package %s is not running", packageName)
}

// PackageLogs reads the current device log buffer for one running app.
func PackageLogs(ctx context.Context, sdks []SDK, nativeID, packageName string) (string, int, error) {
	pid, err := PackagePID(ctx, sdks, nativeID, packageName)
	if err != nil {
		return "", 0, err
	}
	adb, ok := findADB(sdks)
	if !ok {
		return "", 0, fmt.Errorf("Android Debug Bridge was not found")
	}
	result, err := system.Run(ctx, adb, []string{"-s", nativeID, "logcat", "-d", "--pid=" + strconv.Itoa(pid)}, nil, "", "")
	if err != nil {
		return "", 0, fmt.Errorf("read Android logs: %w: %s", err, strings.TrimSpace(result.Output))
	}
	return result.Output, pid, nil
}

func FollowPackageLogs(ctx context.Context, sdks []SDK, nativeID, packageName string, output io.Writer) (int, error) {
	return FollowPackageLogsLines(ctx, sdks, nativeID, packageName, func(line string) error {
		_, err := io.WriteString(output, line+"\n")
		return err
	})
}

// FollowPackageLogsLines follows logcat for an application's current process.
// Each complete line is delivered to the callback so callers can preserve their
// own output protocol, including JSON Lines event streams.
func FollowPackageLogsLines(ctx context.Context, sdks []SDK, nativeID, packageName string, receive func(string) error) (int, error) {
	pid, err := PackagePID(ctx, sdks, nativeID, packageName)
	if err != nil {
		return 0, err
	}
	adb, ok := findADB(sdks)
	if !ok {
		return 0, fmt.Errorf("Android Debug Bridge was not found")
	}
	command := exec.CommandContext(ctx, adb, "-s", nativeID, "logcat", "--pid="+strconv.Itoa(pid))
	stdout, err := command.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("follow Android logs: %w", err)
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return 0, fmt.Errorf("follow Android logs: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := receive(scanner.Text()); err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return 0, fmt.Errorf("follow Android logs: write output: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return 0, fmt.Errorf("follow Android logs: %w", err)
	}
	if err := command.Wait(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return 0, fmt.Errorf("follow Android logs: %w: %s", err, detail)
		}
		return 0, fmt.Errorf("follow Android logs: %w", err)
	}
	return pid, nil
}

// MirrorDevice launches a supported external Android mirroring client for one
// already-connected device. Mob delegates video and input transport entirely
// to the client rather than reimplementing it in a terminal or VS Code view.
func MirrorDevice(ctx context.Context, nativeID string) error {
	client, found := system.LookPath("scrcpy")
	if !found {
		return fmt.Errorf("scrcpy was not found on PATH")
	}
	return MirrorDeviceWithClient(ctx, nativeID, client)
}

// MirrorDeviceWithClient launches the specified verified mirror runtime.
func MirrorDeviceWithClient(ctx context.Context, nativeID, client string) error {
	if strings.TrimSpace(client) == "" {
		return fmt.Errorf("scrcpy client path is required")
	}
	if err := system.Start(ctx, client, mirrorClientArgs(nativeID), nil, ""); err != nil {
		return fmt.Errorf("start Android device mirror: %w", err)
	}
	return nil
}

// mirrorClientArgs makes the mirror safe to use alongside normal desktop
// context-menu habits. scrcpy's SDK mouse default maps a right click to the
// Android Back key, which can unexpectedly close the foreground application.
// Keep the remaining secondary shortcuts while leaving right click unbound.
func mirrorClientArgs(nativeID string) []string {
	return []string{"--serial", nativeID, "--mouse-bind=+hsn:++++"}
}

func containsPID(output string, expected int) bool {
	for _, value := range strings.Fields(output) {
		pid, err := strconv.Atoi(value)
		if err == nil && pid == expected {
			return true
		}
	}
	return false
}

// ParseDevices accepts the stable output of "adb devices -l" without relying
// on localized diagnostics that may precede the device table.
func ParseDevices(output string) []Device {
	devices := make([]Device, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "List" || strings.HasPrefix(fields[0], "*") {
			continue
		}
		nativeID, deviceState := fields[0], fields[1]
		if strings.Contains(nativeID, ":") && deviceState == "daemon" {
			continue
		}
		properties := make(map[string]string)
		for _, field := range fields[2:] {
			if key, value, found := strings.Cut(field, ":"); found {
				properties[key] = value
			}
		}
		kind := "physical"
		if strings.HasPrefix(nativeID, "emulator-") {
			kind = "emulator"
		}
		name := properties["model"]
		if name == "" {
			name = nativeID
		}
		name = strings.ReplaceAll(name, "_", " ")
		devices = append(devices, Device{ID: "android:" + nativeID, Platform: "android", NativeID: nativeID, Kind: kind, Name: name, State: normalizeDeviceState(deviceState)})
	}
	return devices
}

func findADB(sdks []SDK) (string, bool) {
	for _, sdk := range sdks {
		if !sdk.Current && hasCurrent(sdks) {
			continue
		}
		path := filepath.Join(sdk.Path, "platform-tools", adbExecutable())
		if systemPath, err := filepath.Abs(path); err == nil {
			if _, found := system.LookPath(systemPath); found {
				return systemPath, true
			}
		}
	}
	if path, found := system.LookPath(adbExecutable()); found {
		return path, true
	}
	return "", false
}

func findEmulator(sdks []SDK) (string, bool) {
	for _, sdk := range sdks {
		if !sdk.Current && hasCurrent(sdks) {
			continue
		}
		path := filepath.Join(sdk.Path, "emulator", emulatorExecutable())
		if systemPath, err := filepath.Abs(path); err == nil {
			if _, found := system.LookPath(systemPath); found {
				return systemPath, true
			}
		}
	}
	if path, found := system.LookPath(emulatorExecutable()); found {
		return path, true
	}
	return "", false
}

func hasCurrent(sdks []SDK) bool {
	for _, sdk := range sdks {
		if sdk.Current {
			return true
		}
	}
	return false
}
func adbExecutable() string {
	if runtime.GOOS == "windows" {
		return "adb.exe"
	}
	return "adb"
}
func emulatorExecutable() string {
	if runtime.GOOS == "windows" {
		return "emulator.exe"
	}
	return "emulator"
}
func normalizeDeviceState(value string) string {
	if value == "device" {
		return "ready"
	}
	return value
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
