package main

import (
	"encoding/json"
	"os"
)

// The list commands print these with -json, which is what the web panel reads. Slices are never nil, so an empty
// result is [] and not null.
type siteRow struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	User       string   `json:"user"`
	Hostnames  []string `json:"hostnames"`
	Detail     string   `json:"detail"`
	TLS        bool     `json:"tls"`
	Root       string   `json:"root"`
	WWW        string   `json:"www"`
	AllowHTTP  bool     `json:"allow_http"`
	CertExpiry int64    `json:"cert_expiry,omitempty"` // when the certificate ends, for a site with HTTPS
}

type dbRow struct {
	Name string `json:"name"`
	Site string `json:"site"`
}

type backupRow struct {
	Site      string   `json:"site"`
	Backup    string   `json:"backup"`
	State     string   `json:"state"` // ok, incomplete or unreadable
	Files     bool     `json:"files"`
	Databases []string `json:"databases"`
	SizeMB    float64  `json:"size_mb"`
}

func printJSON(v any) { fail(json.NewEncoder(os.Stdout).Encode(v)) }
