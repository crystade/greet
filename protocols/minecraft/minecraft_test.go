package minecraft

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestWriteReadVarInt(t *testing.T) {
	tests := []int32{0, 1, 127, 128, 255, 25565, 2097151, -1}

	for _, want := range tests {
		var buf bytes.Buffer
		if err := writeVarInt(&buf, want); err != nil {
			t.Errorf("writeVarInt(%d): %v", want, err)
			continue
		}

		got, err := readVarInt(&buf)
		if err != nil {
			t.Errorf("readVarInt after writeVarInt(%d): %v", want, err)
			continue
		}
		if got != want {
			t.Errorf("VarInt round-trip: wrote %d, read %d", want, got)
		}
	}
}

func TestVarIntEncoding(t *testing.T) {
	// 0 → [0x00]
	var buf bytes.Buffer
	writeVarInt(&buf, 0)
	if buf.Bytes()[0] != 0x00 {
		t.Errorf("VarInt(0) = 0x%02x, want 0x00", buf.Bytes()[0])
	}

	// 1 → [0x01]
	buf.Reset()
	writeVarInt(&buf, 1)
	if buf.Bytes()[0] != 0x01 {
		t.Errorf("VarInt(1) = 0x%02x, want 0x01", buf.Bytes()[0])
	}

	// 128 → [0x80, 0x01]
	buf.Reset()
	writeVarInt(&buf, 128)
	if len(buf.Bytes()) != 2 || buf.Bytes()[0] != 0x80 || buf.Bytes()[1] != 0x01 {
		t.Errorf("VarInt(128) = %v, want [0x80, 0x01]", buf.Bytes())
	}

	// 25565 → [0xdd, 0xc7, 0x01]
	buf.Reset()
	writeVarInt(&buf, 25565)
	expected := []byte{0xdd, 0xc7, 0x01}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("VarInt(25565) = %v, want %v", buf.Bytes(), expected)
	}
}

func TestWriteString(t *testing.T) {
	var buf bytes.Buffer
	if err := writeString(&buf, "hello"); err != nil {
		t.Fatalf("writeString(hello): %v", err)
	}

	// Should be: VarInt(5) + "hello"
	if buf.Len() != 6 {
		t.Fatalf("writeString(hello) length = %d, want 6", buf.Len())
	}
	if buf.Bytes()[0] != 5 {
		t.Errorf("writeString(hello) first byte = %d, want 5", buf.Bytes()[0])
	}
	if string(buf.Bytes()[1:]) != "hello" {
		t.Errorf("writeString(hello) content = %q, want %q", string(buf.Bytes()[1:]), "hello")
	}
}

func TestExtractMOTD(t *testing.T) {
	// Plain string
	got := extractMOTD([]byte(`"A Minecraft Server"`))
	if got != "A Minecraft Server" {
		t.Errorf("extractMOTD plain string = %q, want %q", got, "A Minecraft Server")
	}

	// Complex object
	got = extractMOTD([]byte(`{"text":"Hello World"}`))
	if got != "Hello World" {
		t.Errorf("extractMOTD complex object = %q, want %q", got, "Hello World")
	}
}

func TestConstants(t *testing.T) {
	if DefaultMinecraftPort != 25565 {
		t.Errorf("DefaultMinecraftPort = %d, want 25565", DefaultMinecraftPort)
	}
	if HandshakePacketID != 0x00 {
		t.Errorf("HandshakePacketID = 0x%02x, want 0x00", HandshakePacketID)
	}
	if NextStateStatus != 1 {
		t.Errorf("NextStateStatus = %d, want 1", NextStateStatus)
	}
	if VarIntSegmentBits != 0x7F {
		t.Errorf("VarIntSegmentBits = 0x%02x, want 0x7F", VarIntSegmentBits)
	}
	if VarIntContinueBit != 0x80 {
		t.Errorf("VarIntContinueBit = 0x%02x, want 0x80", VarIntContinueBit)
	}
}

func TestWriteVarIntError(t *testing.T) {
	// writeVarInt to a failing writer should return an error
	err := writeVarInt(&failingWriter{}, 42)
	if err == nil {
		t.Error("writeVarInt to failing writer should return error")
	}
}

func TestWriteStringError(t *testing.T) {
	// writeString to a failing writer (on the length VarInt) should return an error
	err := writeString(&failingWriter{}, "hello")
	if err == nil {
		t.Error("writeString to failing writer should return error")
	}
}

func TestReadVarIntTruncated(t *testing.T) {
	// Empty reader
	_, err := readVarInt(bytes.NewReader(nil))
	if err == nil {
		t.Error("readVarInt with empty reader should return error")
	}

	// Single byte 0x80 with continuation bit set but no more data
	_, err = readVarInt(bytes.NewReader([]byte{0x80}))
	if err == nil {
		t.Error("readVarInt with truncated data should return error")
	}
}

func TestReadVarIntTooBig(t *testing.T) {
	// 5 bytes all with continuation bit set = 35 bits > 32
	data := []byte{0x80, 0x80, 0x80, 0x80, 0x80}
	_, err := readVarInt(bytes.NewReader(data))
	if err == nil {
		t.Error("readVarInt with 5+ bytes should return error")
	}
	if !strings.Contains(err.Error(), "VarInt too big") {
		t.Errorf("expected 'VarInt too big', got %v", err)
	}
}

// failingWriter always returns an error on Write.
type failingWriter struct{}

func (f *failingWriter) Write([]byte) (int, error) {
	return 0, io.ErrShortWrite
}
