// Package ssh provides the JSON-serializable model and mapper for the SSH
// layer's result. It registers itself with the jsonapi registry via init().
package ssh

import (
	"github.com/crystade/greet/jsonapi"
	"github.com/crystade/greet/protocols/ssh"
)

// Data is the JSON-serializable payload for an SSH layer.
type Data struct {
	VersionString string `json:"versionString"`
}

// FromResult maps a *ssh.SSHResult to *Data. It returns the input unchanged
// when data is not a *ssh.SSHResult.
func FromResult(data any) any {
	r, ok := data.(*ssh.SSHResult)
	if !ok || r == nil {
		return data
	}
	return &Data{VersionString: r.VersionString}
}

func init() {
	jsonapi.RegisterDataMapper(ssh.ProtocolName, FromResult)
}
