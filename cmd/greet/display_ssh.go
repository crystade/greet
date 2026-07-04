package main

import (
	"fmt"

	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/ssh"
)

func printSSHData(data *ssh.SSHResult) {
	fmt.Printf("Version String: %s\n", data.VersionString)
}

func init() {
	registerLayerPrinter("ssh", func(layer greet.LayerResult) {
		if s, ok := layer.Data.(*ssh.SSHResult); ok {
			printSSHData(s)
		}
	})
}
