package cfdns

// ValidToken reports whether t looks like a Cloudflare API token: letters, digits, "_" and "-", 20 to 128 of them. The panel checks
// this before it sends the token anywhere, so a line break or a space cannot reach a header, and a pasted sentence is refused at once.
func ValidToken(t string) bool { return tokenRe.MatchString(t) }
