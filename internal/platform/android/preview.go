package android

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xy200303/MobBase/internal/device/session"
	"github.com/xy200303/MobBase/internal/system"
)

const (
	previewRemoteServer  = "/data/local/tmp/mob-scrcpy-server.jar"
	previewPacketConfig  = uint64(1) << 62
	previewPacketKey     = uint64(1) << 61
	previewPTSBitMask    = previewPacketKey - 1
	previewMaxPacketSize = 16 * 1024 * 1024
)

// PreviewSession exposes a short-lived, loopback-only Android H.264 preview.
// It is deliberately transport-neutral so an IDE, terminal client, or test
// harness can use the same video and input channel without accessing ADB.
type PreviewSession struct {
	Endpoint string
	Token    string

	ctx            context.Context
	cancel         context.CancelFunc
	server         *http.Server
	listener       net.Listener
	streamListener net.Listener
	stream         net.Conn
	adb            string
	nativeID       string
	socketName     string
	serverOutput   previewLog
	peers          *previewPeers

	done     chan struct{}
	doneOnce sync.Once
	errMu    sync.Mutex
	err      error
}

type previewLog struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (l *previewLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.Write(data)
}

func (l *previewLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buffer.String()
}

// StartPreview creates a private video/control service backed by Mob's
// scrcpy-server H.264 socket. Neither listener is exposed beyond localhost.
func StartPreview(ctx context.Context, sdks []SDK, nativeID, serverPath, serverVersion string) (*PreviewSession, error) {
	adb, ok := findADB(sdks)
	if !ok {
		return nil, fmt.Errorf("Android Debug Bridge was not found")
	}
	if strings.TrimSpace(serverPath) == "" || strings.TrimSpace(serverVersion) == "" {
		return nil, fmt.Errorf("Mob Android preview runtime is incomplete")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for Android preview: %w", err)
	}
	streamListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("listen for Android preview video: %w", err)
	}
	token, err := previewToken()
	if err != nil {
		listener.Close()
		streamListener.Close()
		return nil, err
	}
	scid, err := previewSCID()
	if err != nil {
		listener.Close()
		streamListener.Close()
		return nil, err
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &PreviewSession{
		Endpoint:       "http://" + listener.Addr().String(),
		Token:          token,
		ctx:            sessionCtx,
		cancel:         cancel,
		listener:       listener,
		streamListener: streamListener,
		adb:            adb,
		nativeID:       nativeID,
		socketName:     "scrcpy_" + scid,
		peers:          newPreviewPeers(),
		done:           make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/video", session.videoHandler)
	mux.HandleFunc("/control", session.controlHandler(sdks, nativeID))
	session.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		err := session.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			session.fail(fmt.Errorf("serve Android preview: %w", err))
		}
	}()
	go func() {
		<-sessionCtx.Done()
		session.close()
	}()
	if err := session.startScrcpyStream(serverPath, serverVersion); err != nil {
		session.fail(err)
		return nil, err
	}
	return session, nil
}

func previewSCID() (string, error) {
	value := make([]byte, 4)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate Android preview stream ID: %w", err)
	}
	// scrcpy accepts a non-negative 31-bit identifier.
	value[0] &= 0x7f
	return fmt.Sprintf("%02x%02x%02x%02x", value[0], value[1], value[2], value[3]), nil
}

// Metadata is the only credential-bearing value returned to an initiating
// client. Callers must not log or persist the token.
func (s *PreviewSession) Metadata() session.Metadata {
	return session.Metadata{
		Protocol: session.ProtocolV1,
		Platform: "android",
		DeviceID: "android:" + s.nativeID,
		Endpoint: s.Endpoint,
		Token:    s.Token,
		Video:    session.Video{Codec: "avc3", Format: "annexb"},
		Controls: []string{session.ControlTap, session.ControlSwipe, session.ControlText, session.ControlKey, session.ControlClose},
	}
}

// Wait blocks until the caller cancels the command or the preview fails.
func (s *PreviewSession) Wait() error {
	<-s.done
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

func (s *PreviewSession) Close() { s.cancel() }

func (s *PreviewSession) close() {
	s.doneOnce.Do(func() {
		s.peers.close()
		if s.stream != nil {
			_ = s.stream.Close()
		}
		if s.streamListener != nil {
			_ = s.streamListener.Close()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
		_ = s.listener.Close()
		s.removeReverse()
		close(s.done)
	})
}

func (s *PreviewSession) removeReverse() {
	if s.adb == "" || s.socketName == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = system.Run(ctx, s.adb, []string{"-s", s.nativeID, "reverse", "--remove", "localabstract:" + s.socketName}, nil, "", "")
}

func (s *PreviewSession) fail(err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		s.errMu.Lock()
		if s.err == nil {
			s.err = err
		}
		s.errMu.Unlock()
	}
	s.cancel()
}

func previewToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate preview session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (s *PreviewSession) authorized(request *http.Request) bool {
	provided := request.URL.Query().Get("token")
	return provided != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(s.Token)) == 1
}

var previewUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 128 * 1024,
	CheckOrigin:     func(*http.Request) bool { return true }, // Token authentication is mandatory.
}

func (s *PreviewSession) videoHandler(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet || !s.authorized(request) {
		http.Error(response, "preview session not authorized", http.StatusUnauthorized)
		return
	}
	connection, err := previewUpgrader.Upgrade(response, request, nil)
	if err != nil {
		return
	}
	peer := s.peers.addVideo(connection)
	defer s.peers.removeVideo(peer)
	peer.writeLoop(s.ctx)
}

func (s *PreviewSession) controlHandler(sdks []SDK, nativeID string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || !s.authorized(request) {
			http.Error(response, "preview session not authorized", http.StatusUnauthorized)
			return
		}
		connection, err := previewUpgrader.Upgrade(response, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		connection.SetReadLimit(8 * 1024)
		for {
			_, payload, err := connection.ReadMessage()
			if err != nil {
				return
			}
			input, err := parsePreviewInput(payload)
			if err == nil && len(input) == 1 && input[0] == "close" {
				s.Close()
				return
			}
			if err == nil {
				err = Input(s.ctx, sdks, nativeID, input)
			}
			if err != nil {
				_ = connection.WriteJSON(map[string]string{"type": "error", "message": err.Error()})
				continue
			}
			_ = connection.WriteJSON(map[string]string{"type": "accepted"})
		}
	}
}

type previewInput struct {
	Type     string `json:"type"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	X2       int    `json:"x2"`
	Y2       int    `json:"y2"`
	Duration int    `json:"duration"`
	Value    string `json:"value"`
}

func parsePreviewInput(payload []byte) ([]string, error) {
	var input previewInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, fmt.Errorf("parse preview input: %w", err)
	}
	switch input.Type {
	case "close":
		return []string{"close"}, nil
	case "tap":
		if input.X < 0 || input.Y < 0 {
			return nil, fmt.Errorf("preview tap coordinates must be non-negative")
		}
		return []string{"tap", fmt.Sprint(input.X), fmt.Sprint(input.Y)}, nil
	case "swipe":
		if input.X < 0 || input.Y < 0 || input.X2 < 0 || input.Y2 < 0 || input.Duration < 1 || input.Duration > 10_000 {
			return nil, fmt.Errorf("preview swipe values are invalid")
		}
		return []string{"swipe", fmt.Sprint(input.X), fmt.Sprint(input.Y), fmt.Sprint(input.X2), fmt.Sprint(input.Y2), fmt.Sprint(input.Duration)}, nil
	case "text":
		value := strings.TrimSpace(input.Value)
		if value == "" || len(value) > 1024 {
			return nil, fmt.Errorf("preview text must contain at most 1024 characters")
		}
		return []string{"text", strings.ReplaceAll(value, " ", "%s")}, nil
	case "key":
		if !validPreviewKey(input.Value) {
			return nil, fmt.Errorf("preview key is not supported")
		}
		return []string{"key", input.Value}, nil
	default:
		return nil, fmt.Errorf("preview input type is not supported")
	}
}

func validPreviewKey(value string) bool {
	for _, key := range []string{"KEYCODE_BACK", "KEYCODE_HOME", "KEYCODE_APP_SWITCH", "KEYCODE_POWER", "KEYCODE_ENTER", "KEYCODE_DEL"} {
		if value == key {
			return true
		}
	}
	return false
}

func (s *PreviewSession) startScrcpyStream(serverPath, version string) error {
	if _, err := system.Run(s.ctx, s.adb, []string{"-s", s.nativeID, "push", serverPath, previewRemoteServer}, nil, "", ""); err != nil {
		return fmt.Errorf("install Android preview server: %w", err)
	}
	port := s.streamListener.Addr().(*net.TCPAddr).Port
	if _, err := system.Run(s.ctx, s.adb, []string{"-s", s.nativeID, "reverse", "localabstract:" + s.socketName, fmt.Sprintf("tcp:%d", port)}, nil, "", ""); err != nil {
		return fmt.Errorf("create Android preview tunnel: %w", err)
	}
	command := exec.CommandContext(s.ctx, s.adb, "-s", s.nativeID, "shell",
		"CLASSPATH="+previewRemoteServer, "app_process", "/", "com.genymobile.scrcpy.Server", version,
		"scid="+strings.TrimPrefix(s.socketName, "scrcpy_"),
		"log_level=warn", "audio=false", "control=false", "max_size=1280", "max_fps=60",
		"video_bit_rate=8000000", "send_device_meta=false", "send_dummy_byte=false", "send_stream_meta=false", "send_frame_meta=true", "cleanup=false")
	command.Stdout = &s.serverOutput
	command.Stderr = &s.serverOutput
	if err := command.Start(); err != nil {
		s.removeReverse()
		return fmt.Errorf("start Android preview server: %w", err)
	}

	if tcp, ok := s.streamListener.(*net.TCPListener); ok {
		_ = tcp.SetDeadline(time.Now().Add(15 * time.Second))
	}
	stream, err := s.streamListener.Accept()
	if err != nil {
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		_ = command.Process.Kill()
		_ = command.Wait()
		s.removeReverse()
		output := strings.TrimSpace(s.serverOutput.String())
		if output != "" {
			return fmt.Errorf("connect Android preview video stream: %w: %s", err, output)
		}
		return fmt.Errorf("connect Android preview video stream: %w", err)
	}
	if tcp, ok := s.streamListener.(*net.TCPListener); ok {
		_ = tcp.SetDeadline(time.Time{})
	}
	s.stream = stream
	go func() {
		err := s.readScrcpyStream(stream)
		if err != nil && s.ctx.Err() == nil {
			s.fail(err)
		}
	}()
	go func() {
		err := command.Wait()
		if err != nil && s.ctx.Err() == nil {
			output := strings.TrimSpace(s.serverOutput.String())
			if output != "" {
				s.fail(fmt.Errorf("Android preview server ended: %w: %s", err, output))
				return
			}
			s.fail(fmt.Errorf("Android preview server ended: %w", err))
		}
	}()
	return nil
}

func (s *PreviewSession) readScrcpyStream(stream net.Conn) error {
	header := make([]byte, 12)
	for {
		if _, err := io.ReadFull(stream, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("read Android preview frame header: %w", err)
		}
		ptsAndFlags := binary.BigEndian.Uint64(header[:8])
		size := binary.BigEndian.Uint32(header[8:])
		if size == 0 || size > previewMaxPacketSize {
			return fmt.Errorf("Android preview frame has invalid size %d", size)
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(stream, payload); err != nil {
			return fmt.Errorf("read Android preview frame: %w", err)
		}
		if ptsAndFlags&previewPacketConfig != 0 {
			s.peers.configure(payload)
			continue
		}
		s.peers.publish(h264Frame{
			Data: payload, Key: ptsAndFlags&previewPacketKey != 0,
			Timestamp: int64(ptsAndFlags & previewPTSBitMask),
			SPS:       h264ParameterSet(payload, 7), PPS: h264ParameterSet(payload, 8),
		})
	}
}

type previewFrame struct {
	Codec     string
	Key       bool
	Timestamp int64
	Data      []byte
}

type previewPeers struct {
	mu      sync.Mutex
	videos  map[*videoPeer]struct{}
	started map[*videoPeer]bool
	codec   string
	sps     []byte
	pps     []byte
	key     *previewFrame
	frameNo int64
}

func newPreviewPeers() *previewPeers {
	return &previewPeers{videos: make(map[*videoPeer]struct{}), started: make(map[*videoPeer]bool)}
}

func (p *previewPeers) addVideo(connection *websocket.Conn) *videoPeer {
	peer := &videoPeer{connection: connection, frames: make(chan previewFrame, 4)}
	p.mu.Lock()
	p.videos[peer] = struct{}{}
	p.started[peer] = false
	if p.key != nil {
		peer.frames <- clonePreviewFrame(*p.key)
		p.started[peer] = true
	}
	p.mu.Unlock()
	return peer
}

func (p *previewPeers) removeVideo(peer *videoPeer) {
	p.mu.Lock()
	if _, found := p.videos[peer]; found {
		delete(p.videos, peer)
		delete(p.started, peer)
		close(peer.frames)
	}
	p.mu.Unlock()
	if peer.connection != nil {
		_ = peer.connection.Close()
	}
}

func (p *previewPeers) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for peer := range p.videos {
		delete(p.videos, peer)
		delete(p.started, peer)
		close(peer.frames)
		if peer.connection != nil {
			_ = peer.connection.Close()
		}
	}
}

func (p *previewPeers) configure(data []byte) {
	sps := h264ParameterSet(data, 7)
	if len(sps) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sps = sps
	p.pps = h264ParameterSet(data, 8)
	p.codec = codecFromSPS(sps)
}

func (p *previewPeers) publish(frame h264Frame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(frame.SPS) > 0 {
		p.sps = append([]byte(nil), frame.SPS...)
		p.codec = codecFromSPS(frame.SPS)
	}
	if len(frame.PPS) > 0 {
		p.pps = append([]byte(nil), frame.PPS...)
	}
	if p.codec == "" {
		return
	}
	p.frameNo++
	data := append([]byte(nil), frame.Data...)
	if frame.Key && !bytes.Contains(data, p.sps) {
		data = h264ParameterSets(p.sps, p.pps, data)
	}
	timestamp := frame.Timestamp
	if timestamp <= 0 {
		timestamp = p.frameNo * 33_333
	}
	packet := previewFrame{Codec: p.codec, Key: frame.Key, Timestamp: timestamp, Data: data}
	if packet.Key {
		cached := clonePreviewFrame(packet)
		p.key = &cached
	}
	for peer := range p.videos {
		if !p.started[peer] && !packet.Key {
			continue
		}
		if packet.Key {
			p.started[peer] = true
		}
		select {
		case peer.frames <- packet:
		default:
		}
	}
}

func clonePreviewFrame(frame previewFrame) previewFrame {
	frame.Data = append([]byte(nil), frame.Data...)
	return frame
}

type videoPeer struct {
	connection *websocket.Conn
	frames     chan previewFrame
	lastCodec  string
}

func (p *videoPeer) writeLoop(ctx context.Context) {
	defer p.connection.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case frame, open := <-p.frames:
			if !open {
				return
			}
			if p.lastCodec != frame.Codec {
				if err := p.connection.WriteJSON(map[string]string{"type": "video-config", "codec": frame.Codec, "format": "annexb"}); err != nil {
					return
				}
				p.lastCodec = frame.Codec
			}
			packet := make([]byte, 9+len(frame.Data))
			if frame.Key {
				packet[0] = 1
			}
			binary.BigEndian.PutUint64(packet[1:9], uint64(frame.Timestamp))
			copy(packet[9:], frame.Data)
			if err := p.connection.WriteMessage(websocket.BinaryMessage, packet); err != nil {
				return
			}
		}
	}
}

type h264Frame struct {
	Data      []byte
	Key       bool
	Timestamp int64
	SPS       []byte
	PPS       []byte
}

// h264ParameterSet extracts an SPS or PPS NAL from an Annex B access unit.
// scrcpy sends these in its codec configuration packet and some encoders also
// include them directly before an IDR frame.
func h264ParameterSet(data []byte, wanted byte) []byte {
	for offset := 0; ; {
		start, length := findH264StartCode(data, offset)
		if start < 0 {
			return nil
		}
		next, _ := findH264StartCode(data, start+length)
		end := len(data)
		if next >= 0 {
			end = next
		}
		if start+length < end {
			nal := data[start+length : end]
			if nal[0]&0x1f == wanted {
				return append([]byte(nil), nal...)
			}
		}
		if next < 0 {
			return nil
		}
		offset = next
	}
}

type h264AccessUnitParser struct {
	pending []byte
	units   [][]byte
	hasVCL  bool
	sps     []byte
	pps     []byte
}

func newH264AccessUnitParser() *h264AccessUnitParser { return &h264AccessUnitParser{} }

func (p *h264AccessUnitParser) Feed(data []byte) []h264Frame {
	p.pending = append(p.pending, data...)
	frames := []h264Frame{}
	for {
		start, startLength := findH264StartCode(p.pending, 0)
		if start < 0 {
			if len(p.pending) > 4 {
				p.pending = append([]byte(nil), p.pending[len(p.pending)-4:]...)
			}
			return frames
		}
		if start > 0 {
			p.pending = p.pending[start:]
			continue
		}
		next, _ := findH264StartCode(p.pending, startLength)
		if next < 0 {
			return frames
		}
		if frame := p.addNAL(append([]byte(nil), p.pending[startLength:next]...)); frame != nil {
			frames = append(frames, *frame)
		}
		p.pending = p.pending[next:]
	}
}

func (p *h264AccessUnitParser) Flush() []h264Frame {
	frames := []h264Frame{}
	if start, length := findH264StartCode(p.pending, 0); start == 0 && len(p.pending) > length {
		if frame := p.addNAL(append([]byte(nil), p.pending[length:]...)); frame != nil {
			frames = append(frames, *frame)
		}
	}
	if frame := p.flush(); frame != nil {
		frames = append(frames, *frame)
	}
	return frames
}

func (p *h264AccessUnitParser) addNAL(nal []byte) *h264Frame {
	if len(nal) == 0 {
		return nil
	}
	nalType := nal[0] & 0x1f
	isVCL := nalType == 1 || nalType == 5
	if isVCL && p.hasVCL && firstMBSlice(nal) == 0 {
		frame := p.flush()
		p.units = nil
		p.hasVCL = false
		p.units = append(p.units, nal)
		p.hasVCL = true
		return frame
	}
	if nalType == 7 {
		p.sps = append([]byte(nil), nal...)
	}
	if nalType == 8 {
		p.pps = append([]byte(nil), nal...)
	}
	p.units = append(p.units, nal)
	p.hasVCL = p.hasVCL || isVCL
	return nil
}

func (p *h264AccessUnitParser) flush() *h264Frame {
	if !p.hasVCL || len(p.units) == 0 {
		return nil
	}
	frame := h264Frame{SPS: p.sps, PPS: p.pps}
	for _, nal := range p.units {
		frame.Data = append(frame.Data, h264StartCode()...)
		frame.Data = append(frame.Data, nal...)
		if len(nal) > 0 && (nal[0]&0x1f) == 5 {
			frame.Key = true
		}
	}
	return &frame
}

func findH264StartCode(data []byte, offset int) (int, int) {
	for index := offset; index+3 <= len(data); index++ {
		if data[index] == 0 && data[index+1] == 0 && data[index+2] == 1 {
			return index, 3
		}
		if index+4 <= len(data) && data[index] == 0 && data[index+1] == 0 && data[index+2] == 0 && data[index+3] == 1 {
			return index, 4
		}
	}
	return -1, 0
}

func firstMBSlice(nal []byte) uint {
	if len(nal) < 2 {
		return 1
	}
	rbsp := make([]byte, 0, len(nal)-1)
	zeros := 0
	for _, value := range nal[1:] {
		if zeros >= 2 && value == 3 {
			zeros = 0
			continue
		}
		rbsp = append(rbsp, value)
		if value == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	leading, bit := 0, 0
	for bit < len(rbsp)*8 && bitAt(rbsp, bit) == 0 {
		leading++
		bit++
	}
	if leading > 31 || bit >= len(rbsp)*8 {
		return 1
	}
	bit++
	value := uint(0)
	for count := 0; count < leading && bit < len(rbsp)*8; count++ {
		value = value<<1 | uint(bitAt(rbsp, bit))
		bit++
	}
	return (1<<leading - 1) + value
}

func bitAt(data []byte, bit int) byte { return (data[bit/8] >> (7 - bit%8)) & 1 }

func codecFromSPS(sps []byte) string {
	if len(sps) < 4 {
		return "avc3.42E01E"
	}
	return fmt.Sprintf("avc3.%02X%02X%02X", sps[1], sps[2], sps[3])
}

func h264StartCode() []byte { return []byte{0, 0, 0, 1} }

func h264ParameterSets(sps, pps, frame []byte) []byte {
	data := make([]byte, 0, 8+len(sps)+len(pps)+len(frame))
	data = append(data, h264StartCode()...)
	data = append(data, sps...)
	data = append(data, h264StartCode()...)
	data = append(data, pps...)
	return append(data, frame...)
}
