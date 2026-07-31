package android

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/xy200303/MobBase/internal/device/session"
)

func TestPreviewMetadataUsesPlatformNeutralSessionProtocol(t *testing.T) {
	preview := &PreviewSession{Endpoint: "http://127.0.0.1:12345", Token: "test-token", nativeID: "emulator-5554"}
	metadata := preview.Metadata()
	if metadata.Protocol != session.ProtocolV1 || metadata.Platform != "android" || metadata.DeviceID != "android:emulator-5554" {
		t.Fatalf("unexpected preview metadata: %#v", metadata)
	}
	for _, control := range []string{session.ControlTap, session.ControlSwipe, session.ControlText, session.ControlKey, session.ControlClose} {
		if !metadata.Supports(control) {
			t.Fatalf("preview metadata does not declare %q: %#v", control, metadata)
		}
	}
}

func TestPreviewInputValidation(t *testing.T) {
	input, err := parsePreviewInput([]byte(`{"type":"swipe","x":10,"y":20,"x2":30,"y2":40,"duration":180}`))
	if err != nil {
		t.Fatalf("parse swipe: %v", err)
	}
	if got, want := joinInput(input), "swipe 10 20 30 40 180"; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
	if _, err := parsePreviewInput([]byte(`{"type":"key","value":"KEYCODE_VOLUME_UP"}`)); err == nil {
		t.Fatal("unsupported key was accepted")
	}
	if _, err := parsePreviewInput([]byte(`{"type":"tap","x":-1,"y":0}`)); err == nil {
		t.Fatal("negative tap coordinate was accepted")
	}
	if closeInput, err := parsePreviewInput([]byte(`{"type":"close"}`)); err != nil || joinInput(closeInput) != "close" {
		t.Fatalf("close input = %q, %v", closeInput, err)
	}
}

func TestH264AccessUnitParserKeepsParameterSetsWithKeyFrame(t *testing.T) {
	parser := newH264AccessUnitParser()
	stream := append(h264NAL([]byte{0x67, 0x42, 0xe0, 0x1e}), h264NAL([]byte{0x68, 0xce, 0x06, 0xe2})...)
	stream = append(stream, h264NAL([]byte{0x65, 0x80})...)
	stream = append(stream, h264NAL([]byte{0x41, 0x80})...)
	stream = append(stream, h264NAL([]byte{0x41, 0x80})...)
	frames := parser.Feed(stream)
	if len(frames) != 1 {
		t.Fatalf("frame count = %d, want 1", len(frames))
	}
	if !frames[0].Key || codecFromSPS(frames[0].SPS) != "avc3.42E01E" {
		t.Fatalf("unexpected key frame: %#v", frames[0])
	}
	if len(frames[0].PPS) == 0 || len(frames[0].Data) == 0 {
		t.Fatalf("key frame lost codec configuration: %#v", frames[0])
	}
	if trailing := parser.Flush(); len(trailing) != 2 || trailing[0].Key || trailing[1].Key {
		t.Fatalf("unexpected trailing frames: %#v", trailing)
	}
}

func TestScrcpyStreamReadsConfigurationAndFrameMetadata(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	peers := newPreviewPeers()
	session := &PreviewSession{peers: peers}
	done := make(chan error, 1)
	go func() { done <- session.readScrcpyStream(server) }()

	config := append(h264NAL([]byte{0x67, 0x42, 0xe0, 0x1e}), h264NAL([]byte{0x68, 0xce, 0x06, 0xe2})...)
	writeScrcpyPacket(t, client, previewPacketConfig, config)
	writeScrcpyPacket(t, client, previewPacketKey|123_456, h264NAL([]byte{0x65, 0x80}))
	_ = client.Close()
	if err := <-done; err != nil {
		t.Fatalf("read scrcpy stream: %v", err)
	}
	peers.mu.Lock()
	defer peers.mu.Unlock()
	if peers.codec != "avc3.42E01E" || len(peers.sps) == 0 || len(peers.pps) == 0 {
		t.Fatalf("configuration was not retained: codec=%q sps=%x pps=%x", peers.codec, peers.sps, peers.pps)
	}
	if peers.frameNo != 1 {
		t.Fatalf("frame count = %d, want 1", peers.frameNo)
	}
}

func TestPreviewPeerPreservesScrcpyTimestamp(t *testing.T) {
	peers := newPreviewPeers()
	peers.configure(append(h264NAL([]byte{0x67, 0x42, 0xe0, 0x1e}), h264NAL([]byte{0x68, 0xce, 0x06, 0xe2})...))
	peer := &videoPeer{frames: make(chan previewFrame, 1)}
	peers.videos[peer] = struct{}{}
	peers.started[peer] = true
	peers.publish(h264Frame{Data: h264NAL([]byte{0x65, 0x80}), Key: true, Timestamp: 123_456})
	frame := <-peer.frames
	if frame.Timestamp != 123_456 || !frame.Key {
		t.Fatalf("frame metadata = %#v", frame)
	}
}

func TestPreviewPeerReceivesCachedKeyFrameOnConnect(t *testing.T) {
	peers := newPreviewPeers()
	peers.configure(append(h264NAL([]byte{0x67, 0x42, 0xe0, 0x1e}), h264NAL([]byte{0x68, 0xce, 0x06, 0xe2})...))
	peers.publish(h264Frame{Data: h264NAL([]byte{0x65, 0x80}), Key: true, Timestamp: 88})
	peer := peers.addVideo(nil)
	defer peers.removeVideo(peer)
	frame := <-peer.frames
	if !frame.Key || frame.Timestamp != 88 || len(frame.Data) == 0 {
		t.Fatalf("cached key frame = %#v", frame)
	}
}

func writeScrcpyPacket(t *testing.T, connection net.Conn, ptsAndFlags uint64, payload []byte) {
	t.Helper()
	header := make([]byte, 12)
	binary.BigEndian.PutUint64(header[:8], ptsAndFlags)
	binary.BigEndian.PutUint32(header[8:], uint32(len(payload)))
	if _, err := connection.Write(append(header, payload...)); err != nil {
		t.Fatalf("write scrcpy packet: %v", err)
	}
}

func h264NAL(payload []byte) []byte {
	return append(h264StartCode(), payload...)
}

func joinInput(input []string) string {
	value := ""
	for index, part := range input {
		if index > 0 {
			value += " "
		}
		value += part
	}
	return value
}
