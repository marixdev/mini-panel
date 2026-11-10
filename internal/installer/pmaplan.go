package installer

import (
	"fmt"
)

// PMAPlan installs the database browser on a machine whose panel is already running. o needs Target, Panel, PanelPort, Engine and
// the PHP versions that are installed. It does what a fresh install does for the browser and then lets the panel know about it:
// the panel's service gets "-pma-port", and the panel restarts a few seconds later, after the answer to this request has reached
// the person (the restart ends the very process that is waiting for that answer).
func PMAPlan(o Options) (Plan, error) {
	o.Panel = true
	if o.PanelPort == 0 {
		o.PanelPort = 2610
	}
	step, err := pmaStep(o)
	if err != nil {
		return nil, err
	}
	unit := "/etc/systemd/system/mpn-web.service"
	port := fmt.Sprint(PMAPort(o.PanelPort))
	// The flag goes to the end of the ExecStart line, whatever flags the line holds already (the names the panel answers to).
	patch := `grep -q -- ' -pma-port ' ` + unit + ` || sed -i 's|^\(ExecStart=.*\)$|\1 -pma-port ` + port + `|' ` + unit
	return Plan{
		{Name: "database browser: account and key", Actions: pmaPrepareActions()},
		step,
		{Name: "database browser: tell the panel", Actions: []Action{
			Run("sh", "-c", patch),
			Run("systemctl", "daemon-reload"),
			Run("systemd-run", "--quiet", "--collect", "--on-active=4", "--unit=mpn-web-restart", "systemctl", "restart", "mpn-web"),
		}},
	}, nil
}
