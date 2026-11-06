// Package unit renders systemd service units for Node.js apps.
package unit

import (
	"fmt"
	"strings"

	"mpn/internal/site"
)

// NodeRoot is where shared Node.js builds live. Each major version is expected at NodeRoot/<major>/bin/node,
// for example a symlink /opt/node/22 pointing at the installed 22.x release.
const NodeRoot = "/opt/node"

// Name is the unit file name for a site.
func Name(s site.Site) string { return "app-" + s.Name + ".service" }

// Python renders the unit for a python site. The app runs with the interpreter of the site's own virtual environment
// (site.VenvDir), so its packages are never mixed with the system's. memoryMB <= 0 means no memory cap.
func Python(s site.Site, memoryMB int) (string, error) {
	if s.Kind != site.Python {
		return "", fmt.Errorf("site %q is %s, not python", s.Name, s.Kind)
	}
	if err := s.Validate(); err != nil {
		return "", err
	}
	lines := []string{
		"[Unit]",
		"Description=Python app " + s.Name,
		"After=network.target",
		"",
		"[Service]",
		"User=" + s.User,
		"WorkingDirectory=" + s.Root,
		"Environment=PYTHONUNBUFFERED=1",
		fmt.Sprintf("Environment=PORT=%d", s.NodePort),
		fmt.Sprintf("ExecStart=%s/bin/python %s", site.VenvDir(s.User), s.Entry),
		"Restart=always",
		"RestartSec=3",
		"NoNewPrivileges=yes",
		"PrivateTmp=yes",
	}
	if memoryMB > 0 {
		lines = append(lines, fmt.Sprintf("MemoryMax=%dM", memoryMB))
	}
	lines = append(lines, "", "[Install]", "WantedBy=multi-user.target", "")
	return strings.Join(lines, "\n"), nil
}

// Node renders the unit for a node site. memoryMB <= 0 means no memory cap.
// MemoryMax only takes effect on a unified cgroup v2 hierarchy.
func Node(s site.Site, memoryMB int) (string, error) {
	if s.Kind != site.Node {
		return "", fmt.Errorf("site %q is %s, not node", s.Name, s.Kind)
	}
	if err := s.Validate(); err != nil {
		return "", err
	}
	lines := []string{
		"[Unit]",
		"Description=Node.js app " + s.Name,
		"After=network.target",
		"",
		"[Service]",
		"User=" + s.User,
		"WorkingDirectory=" + s.Root,
		"Environment=NODE_ENV=production",
		fmt.Sprintf("Environment=PORT=%d", s.NodePort),
		fmt.Sprintf("ExecStart=%s/%s/bin/node %s", NodeRoot, s.NodeVersion, s.Entry),
		"Restart=always",
		"RestartSec=3",
		"NoNewPrivileges=yes",
		"PrivateTmp=yes",
	}
	if memoryMB > 0 {
		lines = append(lines, fmt.Sprintf("MemoryMax=%dM", memoryMB))
	}
	lines = append(lines, "", "[Install]", "WantedBy=multi-user.target", "")
	return strings.Join(lines, "\n"), nil
}
