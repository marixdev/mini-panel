//go:build !linux

package ctl

import (
	"errors"
	"net"
)

// PeerUID is only available on Linux; elsewhere every caller is refused.
func PeerUID(net.Conn) (uint32, error) { return 0, errors.New("peer credentials need Linux") }
