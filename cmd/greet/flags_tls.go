package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/crystade/greet"
	"github.com/crystade/greet/protocols/tls"
)

func init() {
	registerFlagHandler(tls.ProtocolName, flagHandler{
		register: registerTLSFlags,
		parse:    parseTLSFlags,
	})
}

func registerTLSFlags(fs *flag.FlagSet) {
	fs.String("server-name", "", "TLS SNI hostname override (default: target host)")
	fs.String("client-cert", "", "Path to PEM client certificate for mTLS (key may be bundled)")
	fs.String("client-key", "", "Path to PEM client private key (default: bundled in --client-cert)")
	fs.String("ca-cert", "", "Path to PEM CA certificate for trust verification")
	fs.Bool("insecure", true, "Skip certificate verification (default true)")
}

func parseTLSFlags(fs *flag.FlagSet) ([]greet.GreetOption, error) {
	serverName := ""
	if f := fs.Lookup("server-name"); f != nil {
		serverName = f.Value.String()
	}

	certFile := ""
	if f := fs.Lookup("client-cert"); f != nil {
		certFile = f.Value.String()
	}

	keyFile := ""
	if f := fs.Lookup("client-key"); f != nil {
		keyFile = f.Value.String()
	}

	caFile := ""
	if f := fs.Lookup("ca-cert"); f != nil {
		caFile = f.Value.String()
	}

	insecure := true
	if f := fs.Lookup("insecure"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			if v, ok := getter.Get().(bool); ok {
				insecure = v
			}
		}
	}

	// Read files into bytes (CLI handles files; library callers pass bytes directly).
	var certPEM, keyPEM, caPEM []byte
	if certFile != "" {
		data, err := os.ReadFile(certFile)
		if err != nil {
			return nil, fmt.Errorf("tls: reading client cert %q: %w", certFile, err)
		}
		certPEM = data
	}
	if keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("tls: reading client key %q: %w", keyFile, err)
		}
		keyPEM = data
	}
	if caFile != "" {
		data, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("tls: reading CA cert %q: %w", caFile, err)
		}
		caPEM = data
	}

	return []greet.GreetOption{
		greet.WithLayerConfig(tls.ProtocolName, &tls.TLSConfig{
			ServerName:         serverName,
			CertPEM:            certPEM,
			KeyPEM:             keyPEM,
			CAPEM:              caPEM,
			InsecureSkipVerify: insecure,
		}),
	}, nil
}
