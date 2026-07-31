# Mob Device Session Protocol

## Status

`mob.device.session.v1` is Mob's local, versioned device display and control contract. It is designed for local IDE clients such as the VS Code extension. It does not replace Android ADB, Apple device services, Xcode, HDC, or DevEco; each platform adapter remains responsible for obtaining a display stream and applying an input event through its supported official channel.

Android is the first complete adapter. It uses Mob's matched scrcpy runtime to provide H.264 video, an ADB reverse tunnel, and ADB-backed input. iOS and HarmonyOS may expose the same session contract only after their official tools provide the required display and control capability.

## Session Lifecycle

The initiating client starts a platform-specific session through the stable CLI command:

```text
mob device preview serve <platform:native-id> --json=events
```

The command writes exactly one `preview` JSON Lines event once the session is ready, then remains running until the client sends `close`, the process is cancelled, or the platform stream fails. The CLI owns all platform processes and temporary forwarding rules.

Current Android invocation:

```powershell
mob device preview serve android:emulator-5554 --json=events
```

The endpoint is always bound to `127.0.0.1`. A session token is generated with cryptographic randomness and is valid only while that CLI process is alive. Clients must keep the endpoint and token in memory, must not write either to settings, logs, or workspace files, and must close the session when their preview surface closes.

## Ready Event

The `preview` event contains this data object:

```json
{
  "protocol": "mob.device.session.v1",
  "platform": "android",
  "deviceId": "android:emulator-5554",
  "endpoint": "http://127.0.0.1:49152",
  "token": "ephemeral-session-token",
  "video": { "codec": "avc3", "format": "annexb" },
  "controls": ["tap", "swipe", "text", "key", "close"]
}
```

`platform` identifies the platform adapter, not a promise that every device feature is available. Clients must treat `controls` as the authoritative capability list. They must hide or disable unavailable interaction rather than infer support from the platform name.

## Transport

The v1 transport is two authenticated WebSockets. Convert the `http` endpoint to `ws`, and include the token as a URL query parameter.

```text
GET ws://127.0.0.1:<port>/video?token=<token>
GET ws://127.0.0.1:<port>/control?token=<token>
```

The `video` socket first sends a JSON codec configuration message:

```json
{ "type": "video-config", "codec": "avc3.42E01E", "format": "annexb" }
```

It then sends binary access units in this exact layout:

```text
byte 0       keyframe flag: 1 = key frame, 0 = delta frame
bytes 1..8   unsigned big-endian presentation timestamp in microseconds
bytes 9..N   encoded video access unit
```

For Android v1, access units are H.264 Annex B. Mob includes SPS/PPS with retained key frames, so a client that joins after streaming began can configure and render without waiting for a later encoder refresh. Future codecs and formats require a new `video-config` value and a client capability check; they do not alter the binary header within v1.

## Controls

The `control` socket accepts one JSON object per input operation:

```json
{ "type": "tap", "x": 120, "y": 480 }
{ "type": "swipe", "x": 120, "y": 480, "x2": 120, "y2": 160, "duration": 280 }
{ "type": "text", "value": "Hello" }
{ "type": "key", "value": "KEYCODE_BACK" }
{ "type": "close" }
```

The server answers successful input with `{ "type": "accepted" }`, or `{ "type": "error", "message": "..." }` when the request is invalid or cannot be applied. `close` ends the complete session and is always required in `controls`. v1 coordinates are integer pixels in the encoded video frame's orientation. A client must map its display coordinates to that frame before it sends input.

`key` names and input semantics are adapter-specific. The `controls` list confirms only an operation family. Clients should use a platform-aware command palette for any platform-specific keys and must not assume Android key names work elsewhere.

## Adapter Requirements

A platform adapter may create a v1 session only when it can satisfy all of these requirements:

- A loopback-only endpoint with an ephemeral, authenticated credential.
- A continuous encoded display stream, plus explicit codec and format configuration.
- Exact cleanup of temporary processes, ports, forwarding rules, and device-side helper files when the session ends.
- Accurate capability reporting. Display-only adapters return `controls: ["close"]`.
- Platform-authorized display and input mechanisms. Mob never bypasses platform permissions, device pairing, code signing, or host operating system requirements.

For iOS, a future adapter will be macOS-hosted and based on Apple-supported device and simulator services. iOS device policies may make it display-only or restrict control operations. For HarmonyOS, a future adapter will use HDC/DevEco-supported mechanisms. Android's scrcpy server is an Android implementation detail and must not be used as an iOS or HarmonyOS dependency.

## Compatibility

Clients must reject an unknown `protocol` value, validate that the received `platform` and device ID match the requested device, and check `controls` before sending input. Adapters may add JSON fields and control names in v1; clients must ignore unknown fields and unavailable controls. Changes to endpoint authentication, binary frame layout, or control message meaning require a new protocol version.
