package minecraft

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/crystade/greet"
)

// https://minecraft.wiki/w/Java_Edition_protocol/Packets#Handshake
// https://minecraft.wiki/w/Java_Edition_protocol/Packets#Status_Request
// https://minecraft.wiki/w/Java_Edition_protocol/Packets#Status_Response
// https://github.com/GeyserMC/MCProtocolLib/blob/master/protocol/src/main/java/org/geysermc/mcprotocollib/protocol/packet/handshake/serverbound/ClientIntentionPacket.java

// ProtocolName is the registered name for the Minecraft protocol.
const ProtocolName = "minecraft"

// DefaultMinecraftPort is the well-known port for Minecraft Java Edition.
const DefaultMinecraftPort = 25565

// Handshake packet ID is always 0x00.
const HandshakePacketID = 0x00

// NextStateStatus is the next-state value for status queries.
const NextStateStatus = 1

// StatusRequestPacketID is always 0x00 (empty payload).
const StatusRequestPacketID = 0x00

// VarInt encoding constants.
const (
	VarIntSegmentBits = 0x7F
	VarIntContinueBit = 0x80
)

// MinecraftConfig holds protocol-specific configuration for Minecraft.
type MinecraftConfig struct {
	ProtocolVersion int // e.g. 775 for 26.1
}

// MinecraftResult holds the outcome of a Minecraft status query.
type MinecraftResult struct {
	Version    string
	MOTD       string
	Players    int
	MaxPlayers int
}

// statusResponseJSON is the JSON structure returned by Minecraft servers.
type statusResponseJSON struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
	} `json:"version"`
	Description json.RawMessage `json:"description"`
	Players     struct {
		Online int `json:"online"`
		Max    int `json:"max"`
	} `json:"players"`
}

// Minecraft implements a Minecraft Java Edition status probe.
// It is stateless — all configuration is passed via GreetOption.
type Minecraft struct{}

func (m *Minecraft) Name() string               { return ProtocolName }
func (m *Minecraft) Description() string        { return "Minecraft Java Edition handshake + status request" }
func (m *Minecraft) DefaultPort() int           { return DefaultMinecraftPort }
func (m *Minecraft) Transport() greet.Transport { return greet.TransportTCP }

func (m *Minecraft) RegisterFlags(fs *flag.FlagSet) {
	// Register flags on the FlagSet; ParseFlags reads the values back.
	fs.Int("protocol-version", 775, "Minecraft protocol version (e.g. 775 for 26.1)")
}

func (m *Minecraft) ParseFlags(fs *flag.FlagSet) ([]greet.GreetOption, error) {
	ver := 775
	if f := fs.Lookup("protocol-version"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(int); ok {
				ver = v
			}
		}
	}
	cfg := &MinecraftConfig{ProtocolVersion: ver}
	return []greet.GreetOption{greet.WithProtocolConfig(cfg)}, nil
}

func (m *Minecraft) Greet(ctx context.Context, host string, port int, opts ...greet.GreetOption) (*greet.GreetResult, error) {
	cfg := greet.ResolveOptions(opts...)

	// Resolve protocol-specific config
	protoVersion := 775
	if cfg.ProtocolConfig != nil {
		mcCfg, ok := cfg.ProtocolConfig.(*MinecraftConfig)
		if !ok {
			return nil, &greet.GreetError{
				Code:     greet.ErrInvalidConfig,
				Message:  fmt.Sprintf("invalid protocol config: expected *MinecraftConfig, got %T", cfg.ProtocolConfig),
				Protocol: ProtocolName,
			}
		}
		protoVersion = mcCfg.ProtocolVersion
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, classifyTCPError(err, host, port)
	}
	defer conn.Close()

	deadline, ok := ctx.Deadline()
	if ok {
		conn.SetDeadline(deadline)
	}

	br := bufio.NewReader(conn)

	// Send Handshake packet
	if err := sendHandshake(conn, host, port, protoVersion); err != nil {
		return nil, err
	}

	// Send Status Request packet
	if err := sendStatusRequest(conn); err != nil {
		return nil, err
	}

	// Read Status Response packet
	result, err := readStatusResponse(br)
	latency := time.Since(start)
	if err != nil {
		return nil, err
	}

	return greet.NewResult(ProtocolName, greet.TransportTCP, latency, true, result), nil
}

// sendHandshake builds and sends the Handshake packet.
func sendHandshake(conn net.Conn, host string, port int, protoVersion int) error {
	// Build payload: PacketID + ProtocolVersion + ServerAddress + ServerPort + NextState
	var payload bytes.Buffer
	writeVarInt(&payload, HandshakePacketID)
	writeVarInt(&payload, int32(protoVersion))
	writeString(&payload, host)
	binary.Write(&payload, binary.BigEndian, uint16(port))
	writeVarInt(&payload, NextStateStatus)

	// Write packet: length + payload
	if err := writeVarInt(conn, int32(payload.Len())); err != nil {
		return &greet.GreetError{
			Code:     classifyWriteErrorCode(err),
			Message:  fmt.Sprintf("failed to send handshake length to %s:%d: %v", host, port, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}
	_, err := conn.Write(payload.Bytes())
	if err != nil {
		return &greet.GreetError{
			Code:     classifyWriteErrorCode(err),
			Message:  fmt.Sprintf("failed to send handshake to %s:%d: %v", host, port, err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}
	return nil
}

// sendStatusRequest sends the empty Status Request packet.
func sendStatusRequest(conn net.Conn) error {
	if err := writeVarInt(conn, 1); err != nil { // packet length = 1
		return &greet.GreetError{
			Code:     classifyWriteErrorCode(err),
			Message:  "failed to send status request length",
			Protocol: ProtocolName,
			Cause:    err,
		}
	}
	_, err := conn.Write([]byte{StatusRequestPacketID})
	if err != nil {
		return &greet.GreetError{
			Code:     classifyWriteErrorCode(err),
			Message:  "failed to send status request",
			Protocol: ProtocolName,
			Cause:    err,
		}
	}
	return nil
}

// classifyWriteErrorCode returns handshake_timeout if the error is a
// timeout, otherwise handshake_failed.
func classifyWriteErrorCode(err error) greet.ErrorCode {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrMinecraftHandshakeTimeout
	}
	return ErrMinecraftHandshakeFailed
}

// readStatusResponse reads and parses the Status Response packet.
func readStatusResponse(r io.Reader) (*MinecraftResult, error) {
	// Read packet length
	_, err := readVarInt(r)
	if err != nil {
		return nil, &greet.GreetError{
			Code:     ErrMinecraftMalformedResponse,
			Message:  "failed to read response packet length",
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	// Read packet ID
	packetID, err := readVarInt(r)
	if err != nil {
		return nil, &greet.GreetError{
			Code:     ErrMinecraftMalformedResponse,
			Message:  "failed to read response packet ID",
			Protocol: ProtocolName,
			Cause:    err,
		}
	}
	if packetID != 0x00 {
		return nil, &greet.GreetError{
			Code:     ErrMinecraftMalformedResponse,
			Message:  fmt.Sprintf("unexpected packet ID: 0x%02x (expected 0x00)", packetID),
			Protocol: ProtocolName,
		}
	}

	// Read JSON string length
	jsonLen, err := readVarInt(r)
	if err != nil {
		return nil, &greet.GreetError{
			Code:     ErrMinecraftMalformedResponse,
			Message:  "failed to read JSON string length",
			Protocol: ProtocolName,
			Cause:    err,
		}
	}
	if jsonLen <= 0 || jsonLen > 32768 {
		return nil, &greet.GreetError{
			Code:     ErrMinecraftMalformedResponse,
			Message:  fmt.Sprintf("invalid JSON string length: %d", jsonLen),
			Protocol: ProtocolName,
		}
	}

	// Read JSON string
	jsonBytes := make([]byte, jsonLen)
	_, err = io.ReadFull(r, jsonBytes)
	if err != nil {
		return nil, &greet.GreetError{
			Code:     ErrMinecraftMalformedResponse,
			Message:  "failed to read JSON response body",
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	// Parse JSON
	var status statusResponseJSON
	if err := json.Unmarshal(jsonBytes, &status); err != nil {
		return nil, &greet.GreetError{
			Code:     ErrMinecraftMalformedResponse,
			Message:  fmt.Sprintf("failed to parse status JSON: %v", err),
			Protocol: ProtocolName,
			Cause:    err,
		}
	}

	// Extract MOTD — may be a string or a complex object
	motd := extractMOTD(status.Description)

	return &MinecraftResult{
		Version:    status.Version.Name,
		MOTD:       motd,
		Players:    status.Players.Online,
		MaxPlayers: status.Players.Max,
	}, nil
}

// extractMOTD handles the MOTD which can be a plain string or a complex JSON object.
func extractMOTD(raw json.RawMessage) string {
	// Try as plain string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Try as complex text component
	var obj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Text
	}

	return string(raw)
}

// writeVarInt writes a Minecraft VarInt to w.
func writeVarInt(w io.Writer, value int32) error {
	uv := uint32(value)
	for {
		b := byte(uv & VarIntSegmentBits)
		uv >>= 7
		if uv != 0 {
			b |= VarIntContinueBit
		}
		if _, err := w.Write([]byte{b}); err != nil {
			return err
		}
		if uv == 0 {
			break
		}
	}
	return nil
}

// readVarInt reads a Minecraft VarInt from r.
func readVarInt(r io.Reader) (int32, error) {
	var result int32
	var position uint
	buf := make([]byte, 1)
	for {
		_, err := io.ReadFull(r, buf)
		if err != nil {
			return 0, err
		}
		b := buf[0]
		result |= int32(b&VarIntSegmentBits) << position
		if (b & VarIntContinueBit) == 0 {
			break
		}
		position += 7
		if position >= 32 {
			return 0, fmt.Errorf("VarInt too big")
		}
	}
	return result, nil
}

// writeString writes a Minecraft protocol string (VarInt length + UTF-8 bytes).
func writeString(w io.Writer, s string) error {
	if err := writeVarInt(w, int32(len(s))); err != nil {
		return err
	}
	_, err := w.Write([]byte(s))
	return err
}

func init() {
	greet.Register(&Minecraft{})
}
