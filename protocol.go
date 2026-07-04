package greet

// Transport is the network transport a layer operates over.
type Transport string

const (
	TransportTCP Transport = "tcp"
	TransportUDP Transport = "udp"
)

// String returns the human-readable transport name.
func (t Transport) String() string { return string(t) }
