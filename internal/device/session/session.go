// Package session defines the platform-neutral contract between a Mob device
// adapter and an IDE or other local preview client.
package session

const ProtocolV1 = "mob.device.session.v1"

const (
	ControlTap   = "tap"
	ControlSwipe = "swipe"
	ControlText  = "text"
	ControlKey   = "key"
	ControlClose = "close"
)

// Video describes the encoded media delivered by the video WebSocket. Codec
// configuration is sent on that socket before binary access units begin.
type Video struct {
	Codec  string `json:"codec"`
	Format string `json:"format"`
}

// Metadata is emitted as the credential-bearing preview event. The endpoint
// is always loopback-only and the token is valid only for this short-lived
// device session. Callers must never persist either field.
type Metadata struct {
	Protocol string   `json:"protocol"`
	Platform string   `json:"platform"`
	DeviceID string   `json:"deviceId"`
	Endpoint string   `json:"endpoint"`
	Token    string   `json:"token"`
	Video    Video    `json:"video"`
	Controls []string `json:"controls"`
}

// Supports reports whether a client may send one of the declared controls.
func (m Metadata) Supports(control string) bool {
	for _, available := range m.Controls {
		if available == control {
			return true
		}
	}
	return false
}
