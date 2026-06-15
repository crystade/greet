package ssh

import (
	"testing"
)

func TestDefaultSSHPort(t *testing.T) {
	if DefaultSSHPort != 22 {
		t.Errorf("DefaultSSHPort = %d, want 22", DefaultSSHPort)
	}
}
