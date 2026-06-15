package udp

import (
	"testing"
)

func TestDefaultUDPPort(t *testing.T) {
	if DefaultUDPPort != 53 {
		t.Errorf("DefaultUDPPort = %d, want 53", DefaultUDPPort)
	}
}
