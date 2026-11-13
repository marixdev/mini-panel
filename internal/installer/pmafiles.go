package installer

import (
	"fmt"
	"strings"

	"mpn/internal/distro"
)

// PMAFiles are the files of the database browser that depend on how this machine is set up: its configuration and the two pages of the
// sign-on, by path. They are what a first install writes. mpn panel sync writes them again when they differ, so a machine whose browser
// was installed before one of them changed (as /pma/ did) gets the change without installing the browser again.
func PMAFiles(panelPort int) map[string]string {
	repl := strings.NewReplacer("@BACKEND@", fmt.Sprint(PanelBackendPort), "@PANELPORT@", fmt.Sprint(panelPort))
	return map[string]string{
		pmaDir + "/config.inc.php": pmaConfig(),
		pmaDir + "/signon.php":     repl.Replace(pmaSignon),
		pmaDir + "/logout.php":     repl.Replace(pmaLogout),
	}
}

// PMAPHPService is the service of PHP-FPM that runs the browser; it is told to read the files again after they are replaced.
func PMAPHPService() string { return distro.PHPService(pmaPHP) }
