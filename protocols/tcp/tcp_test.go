package tcp

import (
	"testing"
)

func TestDefaultTCPPort(t *testing.T) {
	if DefaultTCPPort != 80 {
		t.Errorf("DefaultTCPPort = %d, want 80", DefaultTCPPort)
	}
}
