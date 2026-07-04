// Package postgresql provides the JSON-serializable model, mapper, and option
// decoder for the PostgreSQL layer. It registers itself with the jsonapi
// registry via init().
package postgresql

import (
	"encoding/json"

	"github.com/crystade/greet"
	"github.com/crystade/greet/jsonapi"
	"github.com/crystade/greet/protocols/postgresql"
)

// Data is the JSON-serializable payload for a PostgreSQL layer.
type Data struct {
	SSLSupported bool `json:"sslSupported"`
}

// Options holds JSON-serializable PostgreSQL layer options.
type Options struct {
	SSLMode string `json:"sslMode"`
}

// FromResult maps a *postgresql.PostgreSQLResult to *Data. It returns the input
// unchanged when data is not a *postgresql.PostgreSQLResult.
func FromResult(data any) any {
	r, ok := data.(*postgresql.PostgreSQLResult)
	if !ok || r == nil {
		return data
	}
	return &Data{SSLSupported: r.SSLSupported}
}

// DecodeOptions decodes raw PostgreSQL options into engine GreetOptions.
func DecodeOptions(raw json.RawMessage) ([]greet.GreetOption, error) {
	var o Options
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, err
	}
	return []greet.GreetOption{
		greet.WithLayerConfig(postgresql.ProtocolName, &postgresql.PostgreSQLConfig{SSLMode: o.SSLMode}),
	}, nil
}

func init() {
	jsonapi.RegisterDataMapper(postgresql.ProtocolName, FromResult)
	jsonapi.RegisterOptionDecoder(postgresql.ProtocolName, DecodeOptions)
}
