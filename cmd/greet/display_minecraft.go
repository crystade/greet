package main

import (
	"fmt"

	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/minecraft"
)

func printMinecraftData(data *minecraft.MinecraftResult) {
	fmt.Printf("Version: %s\n", data.Version)
	fmt.Printf("MOTD: %s\n", data.MOTD)
	fmt.Printf("Players Online: %d\n", data.Players)
	fmt.Printf("Players Max: %d\n", data.MaxPlayers)
}

func init() {
	registerDataPrinter("minecraft", func(result *greet.GreetResult) {
		if mc, ok := result.Data.(*minecraft.MinecraftResult); ok {
			printMinecraftData(mc)
		}
	})
}
