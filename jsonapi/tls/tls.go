// Package tls provides the JSON-serializable model and mapper for the TLS
// layer's result. It registers itself with the jsonapi registry via init().
package tls

import (
	"github.com/crystade/greet"
	"github.com/crystade/greet/jsonapi"
	"github.com/crystade/greet/protocols/tls"
)

// Data is the JSON-serializable payload for a TLS layer.
type Data struct {
	CertChain []CertChainEntry `json:"certChain"`
	Status    []string         `json:"status"`
}

// CertChainEntry is the JSON-serializable form of a single certificate.
type CertChainEntry struct {
	Subject           string   `json:"subject"`
	Issuer            string   `json:"issuer"`
	Serial            string   `json:"serial"`
	NotBefore         string   `json:"notBefore"`
	NotAfter          string   `json:"notAfter"`
	Version           int      `json:"version"`
	DNSNames          []string `json:"dnsNames,omitempty"`
	IPAddresses       []string `json:"ipAddresses,omitempty"`
	IsCA              bool     `json:"isCa"`
	SignatureAlgo     string   `json:"signatureAlgo"`
	PublicKeyAlgo     string   `json:"publicKeyAlgo"`
	SHA256Fingerprint string   `json:"sha256Fingerprint"`
	Status            []string `json:"status"`
}

// FromResult maps a tls.TLSResult to its JSON representation.
func FromResult(data greet.LayerData) any {
	r, ok := data.(*tls.TLSResult)
	if !ok || r == nil {
		return data
	}

	out := &Data{Status: r.Status}
	if len(r.CertChain) > 0 {
		out.CertChain = make([]CertChainEntry, len(r.CertChain))
		for i, c := range r.CertChain {
			out.CertChain[i] = CertChainEntry{
				Subject:           c.Subject,
				Issuer:            c.Issuer,
				Serial:            c.Serial,
				NotBefore:         c.NotBefore,
				NotAfter:          c.NotAfter,
				Version:           c.Version,
				DNSNames:          c.DNSNames,
				IPAddresses:       c.IPAddresses,
				IsCA:              c.IsCA,
				SignatureAlgo:     c.SignatureAlgo,
				PublicKeyAlgo:     c.PublicKeyAlgo,
				SHA256Fingerprint: c.SHA256Fingerprint,
				Status:            c.Status,
			}
		}
	}
	return out
}

// Register registers the TLS data mapper on the given Codec.
func Register(c *jsonapi.Codec) {
	c.RegisterDataMapper(tls.ProtocolName, FromResult)
}
