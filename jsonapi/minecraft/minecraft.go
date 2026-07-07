// Package minecraft provides the JSON-serializable model, mapper, and option
// decoder for the Minecraft layer. It registers itself with the jsonapi
// registry via init().
package minecraft

import (
	"encoding/json"

	"github.com/crystade/greet"
	"github.com/crystade/greet/jsonapi"
	"github.com/crystade/greet/protocols/minecraft"
)

// Data is the JSON-serializable payload for a Minecraft layer.
type Data struct {
	Version    string `json:"version"`
	MOTD       string `json:"motd"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"maxPlayers"`
}

// Options holds JSON-serializable Minecraft layer options.
type Options struct {
	ProtocolVersion int `json:"protocolVersion"`
}

// FromResult maps a *minecraft.MinecraftResult to *Data. It returns the input
// unchanged when data is not a *minecraft.MinecraftResult.
func FromResult(data greet.LayerData) any {
	r, ok := data.(*minecraft.MinecraftResult)
	if !ok || r == nil {
		return data
	}
	return &Data{
		Version:    r.Version,
		MOTD:       r.MOTD,
		Players:    r.Players,
		MaxPlayers: r.MaxPlayers,
	}
}

// DecodeOptions decodes raw Minecraft options into engine GreetOptions.
func DecodeOptions(raw json.RawMessage) ([]greet.GreetOption, error) {
	var o Options
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, err
	}
	return []greet.GreetOption{
		greet.WithLayerConfig(minecraft.ProtocolName, &minecraft.MinecraftConfig{ProtocolVersion: o.ProtocolVersion}),
	}, nil
}

// Register registers the Minecraft data mapper and option decoder on the given Codec.
func Register(c *jsonapi.Codec) {
	c.RegisterDataMapper(minecraft.ProtocolName, FromResult)
	c.RegisterOptionDecoder(minecraft.ProtocolName, DecodeOptions)
}
