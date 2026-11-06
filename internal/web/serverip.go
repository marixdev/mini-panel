package web

import "net"

// serverIP is the address shown in the header of a site, for the A record of its domain. When the panel was opened by IP
// address, that address is the answer. Otherwise it is the address the machine uses to reach the Internet: a UDP "connection"
// only picks a route, nothing is sent. Behind NAT this is the inner address, so the page calls it the server's address, not a
// guarantee of what the world sees.
func serverIP(hostHeader string) string {
	if h := hostOnly(hostHeader); net.ParseIP(h) != nil {
		return h
	}
	c, err := net.Dial("udp4", "192.0.2.1:9") // a documentation address, never routed
	if err != nil {
		return ""
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok && !a.IP.IsLoopback() {
		return a.IP.String()
	}
	return ""
}
