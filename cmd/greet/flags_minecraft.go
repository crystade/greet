package main

import (
	"flag"

	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/minecraft"
)

func init() {
	registerFlagHandler(minecraft.ProtocolName, flagHandler{
		register: registerMinecraftFlags,
		parse:    parseMinecraftFlags,
	})
}

func registerMinecraftFlags(fs *flag.FlagSet) {
	fs.Int("protocol-version", 775, "Minecraft protocol version (e.g. 775 for 26.1)")
}

func parseMinecraftFlags(fs *flag.FlagSet) ([]greet.GreetOption, error) {
	ver := 775
	if f := fs.Lookup("protocol-version"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(int); ok {
				ver = v
			}
		}
	}
	return []greet.GreetOption{
		greet.WithLayerConfig(minecraft.ProtocolName, &minecraft.MinecraftConfig{ProtocolVersion: ver}),
	}, nil
}
