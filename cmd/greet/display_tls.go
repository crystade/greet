package main

import (
	"fmt"
	"strings"

	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/tls"
)

func printTLSData(data *tls.TLSResult) {
	if len(data.CertChain) > 0 {
		leaf := data.CertChain[0]
		fmt.Printf("Subject: %s\n", leaf.Subject)
		fmt.Printf("Issuer: %s\n", leaf.Issuer)
		fmt.Printf("Serial: %s\n", leaf.Serial)
		fmt.Printf("Not Before: %s\n", leaf.NotBefore)
		fmt.Printf("Not After: %s\n", leaf.NotAfter)
		fmt.Printf("Version: %d\n", leaf.Version)
		fmt.Printf("DNS Names: %v\n", leaf.DNSNames)
		fmt.Printf("Signature Algorithm: %s\n", leaf.SignatureAlgo)
		fmt.Printf("Public Key Algorithm: %s\n", leaf.PublicKeyAlgo)
		fmt.Printf("Fingerprint (SHA-256): %s\n", leaf.SHA256Fingerprint)
	}

	fmt.Printf("Chain Status: %s\n", strings.Join(data.Status, ", "))

	if len(data.CertChain) > 0 {
		fmt.Printf("\nCertificate Chain (%d certificates, leaf → root):\n", len(data.CertChain))
		for i, entry := range data.CertChain {
			role := "intermediate"
			if i == 0 {
				role = "leaf"
			} else if i == len(data.CertChain)-1 {
				role = "root"
			}
			fmt.Printf("\n  [%d] (%s)\n", i, role)
			fmt.Printf("      Subject:           %s\n", entry.Subject)
			fmt.Printf("      Issuer:            %s\n", entry.Issuer)
			fmt.Printf("      Serial:            %s\n", entry.Serial)
			fmt.Printf("      Not Before:        %s\n", entry.NotBefore)
			fmt.Printf("      Not After:         %s\n", entry.NotAfter)
			fmt.Printf("      Fingerprint:       %s\n", entry.SHA256Fingerprint)
			fmt.Printf("      Status:            %s\n", strings.Join(entry.Status, ", "))
		}
	}
}

func init() {
	registerDataPrinter("tls", func(result *greet.GreetResult) {
		if r, ok := result.Data.(*tls.TLSResult); ok {
			printTLSData(r)
		}
	})
}
