package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"

	"mpn/internal/executor"
	"mpn/internal/installer"
)

// runPlanQuiet applies a plan and returns the error, for callers that print their own last line.
func runPlanQuiet(plan installer.Plan) error {
	requireLinuxRoot()
	return executor.Apply(plan, &executor.OS{Out: os.Stdout}, os.Stdout)
}

// parseAll reads every certificate in PEM text.
func parseAll(data []byte) ([]*x509.Certificate, error) {
	var out []*x509.Certificate
	for {
		var b *pem.Block
		b, data = pem.Decode(data)
		if b == nil {
			return out, nil
		}
		if b.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
}
