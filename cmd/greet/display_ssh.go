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
	registerDataPrinter("ssh", func(result *greet.GreetResult) {
		if s, ok := result.Data.(*ssh.SSHResult); ok {
			printSSHData(s)
		}
	})
}
