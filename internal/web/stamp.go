package web

import "time"

// shellName maps a shell file to the name of the template it defines.
var shellName = map[string]string{"auth": "auth", "base": "app"}

// navOf says which menu entry is highlighted on a page. Result and confirmation pages highlight none.
var navOf = map[string]string{"sites": "sites", "sitenew": "sites", "site": "sites", "fileedit": "sites", "filerename": "sites", "pmaopen": "databases", "databases": "databases", "backups": "backups", "firewall": "firewall"}

// stampText turns a backup name such as 20261104-031500 into 04/11/2026 03:15:00. Anything else is shown as it is.
func stampText(s string) string {
	t, err := time.Parse("20060102-150405", s)
	if err != nil {
		return s
	}
	return t.Format("02/01/2006 15:04:05")
}
