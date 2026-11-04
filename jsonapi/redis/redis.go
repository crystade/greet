// Package redis provides the JSON-serializable model, mapper, and option
// decoder for the Redis layer. It registers itself with the jsonapi
// registry via init().
package redis

import (
	"encoding/json"

	"github.com/crystade/greet"
	"github.com/crystade/greet/jsonapi"
	greetredis "github.com/crystade/greet/protocols/redis"
)

// Data is the JSON-serializable payload for a Redis layer.
// Pong is set for PING probes; Info is set for HELLO probes
// (info.proto is the negotiated RESP version).
type Data struct {
	Pong bool              `json:"pong"`
	Info map[string]string `json:"info,omitempty"`
}

// Options holds JSON-serializable Redis layer options.
type Options struct {
	UseHello bool `json:"useHello,omitempty"`
	// HelloMode is "auto" (default), "resp2" or "resp3".
	// A non-empty value implies useHello.
	HelloMode string `json:"helloMode,omitempty"`
}

// FromResult maps a *greetredis.RedisResult to *Data. It returns the input
// unchanged when data is not a *greetredis.RedisResult.
func FromResult(data greet.LayerData) any {
	r, ok := data.(*greetredis.RedisResult)
	if !ok || r == nil {
		return data
	}
	return &Data{
		Pong: r.Pong,
		Info: r.Info,
	}
}

// DecodeOptions decodes raw Redis options into engine GreetOptions.
func DecodeOptions(raw json.RawMessage) ([]greet.GreetOption, error) {
	var o Options
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, err
	}

	var mode greetredis.HelloMode
	if o.HelloMode != "" {
		m, err := greetredis.ParseHelloMode(o.HelloMode)
		if err != nil {
			return nil, err
		}
		mode = m
	}

	return []greet.GreetOption{
		greet.WithLayerConfig(greetredis.ProtocolName, &greetredis.RedisConfig{
			UseHello:  o.UseHello,
			HelloMode: mode,
		}),
	}, nil
}

// Register registers the Redis data mapper and option decoder on the given Codec.
func Register(c *jsonapi.Codec) {
	c.RegisterDataMapper(greetredis.ProtocolName, FromResult)
	c.RegisterOptionDecoder(greetredis.ProtocolName, DecodeOptions)
}
