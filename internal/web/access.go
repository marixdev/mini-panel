package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/dbadmin"
	"mpn/internal/site"
	"mpn/internal/webauth"
)

// A rule says who may call one route. The table is keyed by r.Pattern, the pattern the route was registered with. A route that
// has no rule and is not public is refused for delegated users.
type rule struct {
	admin bool         // only the administrator
	min   webauth.Role // the least role the delegated user needs on the website
	src   string       // where the website name comes from: "", "path", "post", "query" or "db"
	field string       // for "post" and "query": the form or query field that names the website
	dbSrc string       // for "db": where the database name comes from, "post" or "query"
	// needPMA means the user also needs the database browser flag (User.PMA) set by the administrator.
	needPMA bool
	// anySite, for "query" rules, lets a request that names no website through: the handler then narrows what the user sees.
	// A request that names a website is still checked against the role on it.
	anySite bool
}

var (
	adminOnly = rule{admin: true}
	anyUser   = rule{} // any signed-in delegated user; the handler narrows what that user sees
	// pmaRule is the database browser: the website is the owner of the database named in the form, and the user needs manage
	// on it and the browser flag.
	pmaRule = rule{min: webauth.RoleManage, src: "db", dbSrc: "post", needPMA: true}
)

func onPath(min webauth.Role) rule { return rule{min: min, src: "path"} }

func onPost(min webauth.Role, field string) rule {
	return rule{min: min, src: "post", field: field}
}

func onQuery(min webauth.Role, field string) rule {
	return rule{min: min, src: "query", field: field}
}

// onDB checks the website that owns the database named in the request (see dbadmin.OwnedBy).
func onDB(min webauth.Role, dbSrc string) rule {
	return rule{min: min, src: "db", dbSrc: dbSrc}
}

// accessRules is the whole policy for signed-in requests. Routes that need no sign-in are in publicRoutes.
var accessRules = map[string]rule{
	// Own pages of any signed-in user.
	"POST /logout": anyUser,
	"GET /result":  anyUser,
	"GET /{$}":     anyUser, // the sites list; the handler shows a delegated user only the websites given to them

	// The Settings page and its actions act on the administrator's account. Delegated users cannot reach them until
	// their own settings are built.
	"GET /settings":               anyUser, // the administrator's account, or a delegated user's own password and 2FA
	"GET /settings/2fa":           anyUser,
	"POST /settings/password":     anyUser,
	"POST /settings/2fa/enable":   anyUser,
	"POST /settings/2fa/disable":  anyUser,
	"POST /settings/sessions/end": anyUser,

	// Website pages (view).
	"GET /sites/{name}":                onPath(webauth.RoleView),
	"GET /sites/{name}/ssl":            onPath(webauth.RoleView),
	"GET /sites/{name}/vhost":          onPath(webauth.RoleView),
	"GET /sites/{name}/databases":      onPath(webauth.RoleView),
	"GET /sites/{name}/backups":        onPath(webauth.RoleView),
	"GET /sites/{name}/cron":           onPath(webauth.RoleView),
	"GET /sites/{name}/background":     onPath(webauth.RoleView),
	"GET /sites/{name}/logs":           onPath(webauth.RoleView),
	"GET /sites/{name}/activity":       onPath(webauth.RoleView),
	"GET /sites/{name}/email":          onPath(webauth.RoleView),
	"GET /sites/{name}/wordpress":      onPath(webauth.RoleView),
	"GET /sites/{name}/ssh":            onPath(webauth.RoleView), // redirects to the overview
	"GET /sites/{name}/files":          onPath(webauth.RoleView),
	"GET /sites/{name}/files/download": onPath(webauth.RoleView),
	"GET /sites/{name}/cloud/backups":  onPath(webauth.RoleView),

	// Website file changes and content (manage).
	"GET /sites/{name}/files/edit":            onPath(webauth.RoleManage),
	"POST /sites/{name}/files/save":           onPath(webauth.RoleManage),
	"POST /sites/{name}/files/upload":         onPath(webauth.RoleManage),
	"POST /sites/{name}/files/mkdir":          onPath(webauth.RoleManage),
	"POST /sites/{name}/files/newfile":        onPath(webauth.RoleManage),
	"GET /sites/{name}/files/rename":          onPath(webauth.RoleManage),
	"POST /sites/{name}/files/rename":         onPath(webauth.RoleManage),
	"POST /sites/{name}/files/confirm-delete": onPath(webauth.RoleManage),
	"POST /sites/{name}/files/delete":         onPath(webauth.RoleManage),
	"POST /sites/{name}/files/zip":            onPath(webauth.RoleManage),
	"POST /sites/{name}/files/chmod":          onPath(webauth.RoleManage),
	"POST /sites/{name}/files/unzip":          onPath(webauth.RoleManage),
	"POST /sites/{name}/files/paste":          onPath(webauth.RoleManage),
	"POST /sites/{name}/cron":                 onPath(webauth.RoleManage),
	"POST /sites/{name}/cron/{id}/toggle":     onPath(webauth.RoleManage),
	"POST /sites/{name}/cron/{id}/delete":     onPath(webauth.RoleManage),
	// A background service runs a command as the site's own account all the time, as a cron job does on a schedule.
	"POST /sites/{name}/background":              onPath(webauth.RoleManage),
	"POST /sites/{name}/background/{id}/edit":    onPath(webauth.RoleManage),
	"POST /sites/{name}/background/{id}/toggle":  onPath(webauth.RoleManage),
	"POST /sites/{name}/background/{id}/restart": onPath(webauth.RoleManage),
	"POST /sites/{name}/background/{id}/delete":  onPath(webauth.RoleManage),
	"POST /sites/{name}/wp/run":                  onPath(webauth.RoleManage),
	"POST /sites/{name}/app/install":             onPath(webauth.RoleManage),
	"POST /sites/{name}/settings":                onPath(webauth.RoleManage),
	"POST /sites/{name}/speed":                   onPath(webauth.RoleManage),
	"POST /sites/{name}/webmail":                 onPath(webauth.RoleManage),

	// Mail of one website (manage).
	"POST /sites/{name}/email/domain/add":    onPath(webauth.RoleManage),
	"POST /sites/{name}/email/domain/delete": onPath(webauth.RoleManage),
	"POST /sites/{name}/email/box/add":       onPath(webauth.RoleManage),
	"POST /sites/{name}/email/box/passwd":    onPath(webauth.RoleManage),
	"POST /sites/{name}/email/box/quota":     onPath(webauth.RoleManage),
	"POST /sites/{name}/email/box/delete":    onPath(webauth.RoleManage),
	"POST /sites/{name}/email/alias/set":     onPath(webauth.RoleManage),
	"POST /sites/{name}/email/alias/delete":  onPath(webauth.RoleManage),
	"GET /sites/{name}/email/cf":             onPath(webauth.RoleManage),
	"POST /sites/{name}/email/cf":            onPath(webauth.RoleManage),

	// Cloud backups of one website (full). Connecting or configuring the storage account is server-wide.
	"POST /sites/{name}/cloud/enable":     onPath(webauth.RoleFull),
	"POST /sites/{name}/cloud/sync":       onPath(webauth.RoleFull),
	"POST /sites/{name}/cloud/fetch":      onPath(webauth.RoleFull),
	"POST /sites/{name}/cloud/connect":    adminOnly,
	"POST /sites/{name}/cloud/configure":  adminOnly,
	"POST /sites/{name}/cloud/finish":     adminOnly,
	"POST /sites/{name}/cloud/disconnect": adminOnly,

	// Full: terminal, SSH/SFTP, SSL, domain, vhost.
	"GET /sites/{name}/terminal":         onPath(webauth.RoleFull),
	"GET /sites/{name}/terminal/ws":      onPath(webauth.RoleFull),
	"POST /sites/{name}/ssh/enable":      onPath(webauth.RoleFull),
	"GET /sites/{name}/ssh/reset":        onPath(webauth.RoleFull),
	"POST /sites/{name}/ssh/reset":       onPath(webauth.RoleFull),
	"GET /sites/{name}/ssh/disable":      onPath(webauth.RoleFull),
	"POST /sites/{name}/ssh/disable":     onPath(webauth.RoleFull),
	"POST /sites/{name}/ssh/panelkey":    onPath(webauth.RoleFull),
	"POST /sites/{name}/vhost":           onPath(webauth.RoleFull),
	"POST /sites/{name}/vhost/reset":     onPath(webauth.RoleFull),
	"POST /sites/{name}/domain":          onPath(webauth.RoleFull),
	"POST /sites/{name}/ssl/letsencrypt": onPath(webauth.RoleFull),
	"POST /sites/{name}/ssl/cloudflare":  onPath(webauth.RoleFull),
	"POST /sites/{name}/ssl/custom":      onPath(webauth.RoleFull),
	"POST /sites/{name}/ssl/csr":         onPath(webauth.RoleFull),
	"POST /sites/{name}/ssl/off":         onPath(webauth.RoleFull),
	"POST /sites/{name}/ssl/redirect":    onPath(webauth.RoleFull),

	// Databases: the database name must belong to the website that is checked (dbadmin.OwnedBy).
	"POST /databases/add":    onPost(webauth.RoleManage, "site"),
	"GET /databases/passwd":  onDB(webauth.RoleManage, "query"),
	"POST /databases/passwd": onDB(webauth.RoleManage, "post"),
	"GET /databases/remove":  onDB(webauth.RoleManage, "query"),
	"POST /databases/remove": onDB(webauth.RoleManage, "post"),
	"GET /databases/remote":  onDB(webauth.RoleFull, "query"),
	"POST /databases/remote": onDB(webauth.RoleFull, "post"),
	"POST /databases/pma":    pmaRule, // the administrator, or a delegated user with the browser flag and manage on the owning website

	// Backups: create and keep are manage, restore is full, prune is server-wide.
	"POST /backups/create":  onPost(webauth.RoleManage, "site"),
	"POST /backups/keep":    onPost(webauth.RoleManage, "site"),
	"GET /backups/restore":  onQuery(webauth.RoleFull, "site"),
	"POST /backups/restore": onPost(webauth.RoleFull, "site"),
	"POST /backups/prune":   adminOnly,

	// Activity: a delegated user who names a website needs a role on it; without one the list is narrowed to the websites
	// they hold a role on, and server-wide entries are left out (see auditPage and auditCSV).
	"GET /activity":     rule{min: webauth.RoleView, src: "query", field: "site", anySite: true},
	"GET /activity.csv": rule{min: webauth.RoleView, src: "query", field: "site", anySite: true},

	// Server-wide pages and the creation and removal of websites: administrator only.
	"GET /sites/new":     adminOnly,
	"POST /sites/add":    adminOnly,
	"POST /sites/apply":  adminOnly,
	"GET /sites/remove":  adminOnly,
	"POST /sites/remove": adminOnly,
	// Delegated accounts: the administrator only. A delegated user cannot manage the accounts, including their own.
	"GET /users":                           adminOnly,
	"POST /users/create":                   adminOnly,
	"POST /users/sites":                    adminOnly,
	"POST /users/password":                 adminOnly,
	"POST /users/disable":                  adminOnly,
	"POST /users/enable":                   adminOnly,
	"POST /users/remove":                   adminOnly,
	"POST /users/2fa-off":                  adminOnly,
	"POST /users/pma":                      adminOnly,
	"GET /update":                          adminOnly,
	"POST /update/check":                   adminOnly,
	"POST /update":                         adminOnly,
	"POST /settings/update/check":          adminOnly,
	"POST /settings/update":                adminOnly,
	"GET /monitor":                         adminOnly,
	"GET /email":                           adminOnly,
	"POST /email/enable":                   adminOnly,
	"POST /email/disable":                  adminOnly,
	"POST /email/ssl":                      adminOnly,
	"POST /email/webmail":                  adminOnly,
	"POST /email/domain/add":               adminOnly,
	"POST /email/domain/delete":            adminOnly,
	"POST /email/box/add":                  adminOnly,
	"POST /email/box/passwd":               adminOnly,
	"POST /email/box/quota":                adminOnly,
	"POST /email/box/delete":               adminOnly,
	"POST /email/alias/set":                adminOnly,
	"POST /email/alias/delete":             adminOnly,
	"GET /email/cf":                        adminOnly,
	"POST /email/cf":                       adminOnly,
	"GET /firewall":                        adminOnly,
	"POST /firewall/enable":                adminOnly,
	"POST /firewall/disable":               adminOnly,
	"POST /firewall/add":                   adminOnly,
	"POST /firewall/block":                 adminOnly,
	"POST /firewall/{id}/delete":           adminOnly,
	"POST /firewall/port":                  adminOnly,
	"POST /firewall/port/finish":           adminOnly,
	"POST /firewall/port/cancel":           adminOnly,
	"GET /security":                        adminOnly,
	"POST /security/waf":                   adminOnly,
	"POST /security/f2b":                   adminOnly,
	"POST /security/unban":                 adminOnly,
	"POST /security/cloudflare":            adminOnly,
	"POST /security/panel-ssl/letsencrypt": adminOnly,
	"POST /security/panel-ssl/custom":      adminOnly,
	"POST /security/panel-ssl/off":         adminOnly,
	"GET /services":                        adminOnly,
	"GET /services/{key}/stop":             adminOnly,
	"GET /services/{key}/remove":           adminOnly,
	"POST /services/{key}/{action}":        adminOnly,
	"GET /services/database/settings":      adminOnly,
	"POST /services/database/settings":     adminOnly,
	"GET /notify":                          adminOnly,
	"POST /notify/hub":                     adminOnly,
	"POST /notify/chat":                    adminOnly,
	"POST /notify/test":                    adminOnly,
	"POST /notify/rules":                   adminOnly,
	"POST /notify/pair":                    adminOnly,
	"POST /notify/agent":                   adminOnly,
	"POST /notify/off":                     adminOnly,
	"POST /notify/noderules":               adminOnly,
	"POST /notify/noderemove":              adminOnly,
}

// publicRoutes need no sign-in. Patterns that begin with one of publicPrefixes are public too: the static files, and the
// alerts that other machines send to the hub (which check their own key).
var publicRoutes = map[string]bool{
	"GET /language/{language}":  true,
	"GET /login":                true,
	"POST /login":               true,
	"POST /login/2fa":           true,
	"GET /setup":                true,
	"POST /setup":               true,
	"GET /":                     true, // the catch-all of unknown addresses
	"GET /_pma/auth":            true,
	"GET /_pma/ticket":          true,
	"GET /_pma/end":             true,
	"GET /static/editor/{file}": true,
}

var publicPrefixes = []string{"GET /static/", "POST /_notify/"}

func isPublic(pattern string) bool {
	if publicRoutes[pattern] {
		return true
	}
	for _, p := range publicPrefixes {
		if strings.HasPrefix(pattern, p) {
			return true
		}
	}
	return false
}

// isAdmin reports whether the session belongs to the administrator's account.
func (s *Server) isAdmin(ss webauth.Session) bool {
	acct, err := s.Account()
	return err == nil && ss.User == acct.User
}

// allowed decides whether a signed-in session may call the route the request is for. The administrator may call everything
// that is registered. A delegated user needs a rule for the route, an enabled account, a website named by the rule that is
// still live, and a role on it at least the rule's. Anything else is refused.
func (s *Server) allowed(r *http.Request, ss webauth.Session) bool {
	if s.isAdmin(ss) {
		return true
	}
	rl, ok := accessRules[r.Pattern]
	if !ok || rl.admin {
		return false
	}
	u, ok := s.delegatedUser(ss.User)
	if !ok {
		return false
	}
	if rl.needPMA && !u.PMA {
		return false
	}
	if rl.src == "" {
		return true
	}
	if rl.anySite && rl.src == "query" && r.URL.Query().Get(rl.field) == "" {
		return true
	}
	siteName, ok := s.siteFor(r, rl)
	if !ok {
		return false
	}
	if !s.siteLive(siteName) {
		return false
	}
	return u.RoleOn(siteName).AtLeast(rl.min)
}

// siteVisible reports whether a session may see a website at all: the administrator sees all, a delegated user sees the
// websites they hold any role on, and only while those websites exist.
func (s *Server) siteVisible(ss webauth.Session, siteName string) bool {
	if s.isAdmin(ss) {
		return true
	}
	u, ok := s.delegatedUser(ss.User)
	return ok && u.RoleOn(siteName) != "" && s.siteLive(siteName)
}

// delegatedUser loads the delegated user named name from the users file, fresh on each call, so a change applies at once.
func (s *Server) delegatedUser(name string) (webauth.User, bool) {
	if s.UsersFile == "" {
		return webauth.User{}, false
	}
	acct, err := s.Account()
	if err != nil {
		return webauth.User{}, false
	}
	users, err := webauth.LoadUsers(s.UsersFile, acct.User)
	if err != nil {
		return webauth.User{}, false
	}
	for _, u := range users {
		if u.Name == name {
			return u, !u.Disabled
		}
	}
	return webauth.User{}, false
}

// siteFor finds the website a request is about, from the rule's source. It returns false when the name is missing or invalid.
func (s *Server) siteFor(r *http.Request, rl rule) (string, bool) {
	var name string
	switch rl.src {
	case "path":
		name = r.PathValue("name")
	case "post":
		name = r.PostFormValue(rl.field)
	case "query":
		name = r.URL.Query().Get(rl.field)
	case "db":
		db := r.PostFormValue("name")
		if rl.dbSrc == "query" {
			db = r.URL.Query().Get("name")
		}
		if !dbadmin.ValidName(db) {
			return "", false
		}
		return s.dbOwner(db)
	default:
		return "", false
	}
	if !site.ValidName(name) {
		return "", false
	}
	return name, true
}

// dbOwner names the website a database belongs to among the live websites: the one whose base name fits and is longest.
func (s *Server) dbOwner(db string) (string, bool) {
	live, err := s.liveSites()
	if err != nil {
		return "", false
	}
	best := ""
	for _, name := range live {
		if dbadmin.OwnedBy(db, name) && len(dbadmin.BaseName(name)) > len(dbadmin.BaseName(best)) {
			best = name
		}
	}
	return best, best != ""
}

// siteLive reports whether the root helper currently lists a website with that name. A role on a website that was removed
// grants nothing.
func (s *Server) siteLive(name string) bool {
	live, err := s.liveSites()
	if err != nil {
		return false
	}
	for _, x := range live {
		if x == name {
			return true
		}
	}
	return false
}

var errNoHelper = errors.New("the root helper did not answer")

// liveSites lists the website names the root helper knows now.
func (s *Server) liveSites() ([]string, error) {
	if s.Call == nil {
		return nil, errNoHelper
	}
	resp, err := s.Call(ctl.Request{Op: "site.list"})
	if err != nil || !resp.OK {
		return nil, errNoHelper
	}
	var rows []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(resp.Output), &rows); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names, nil
}

// deny answers a request the session may not make: 403 with the panel's own not-found page, so the response says nothing
// about whether the website or the page exists. A refused POST is written to the activity log.
func (s *Server) deny(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	if r.Method == http.MethodPost {
		s.note(r, "access.denied", "", "user="+ss.User+" route="+r.Pattern, true)
	}
	s.render(w, http.StatusForbidden, view{Page: "notfound"})
}

// userRecord returns the delegated user named name, whether or not it is disabled, read from the users file. The second return
// value is false when there is no such user or the file cannot be read.
func (s *Server) userRecord(name string) (webauth.User, bool) {
	if s.UsersFile == "" {
		return webauth.User{}, false
	}
	acct, err := s.Account()
	if err != nil {
		return webauth.User{}, false
	}
	users, err := webauth.LoadUsers(s.UsersFile, acct.User)
	if err != nil {
		return webauth.User{}, false
	}
	for _, u := range users {
		if u.Name == name {
			return u, true
		}
	}
	return webauth.User{}, false
}

// saveDelegated writes one delegated user's record back to the users file and leaves the other users as they are.
func (s *Server) saveDelegated(u webauth.User) error {
	acct, err := s.Account()
	if err != nil {
		return err
	}
	users, err := webauth.LoadUsers(s.UsersFile, acct.User)
	if err != nil {
		return err
	}
	for i := range users {
		if users[i].Name == u.Name {
			users[i] = u
		}
	}
	return webauth.SaveUsers(s.UsersFile, users, acct.User, -1, -1)
}

// canFunc returns the function a template calls as .Can. It tells whether the session holds at least the named role on the
// website. The administrator holds every role on every website. Hiding a control this way is cosmetic: the route rules refuse the
// request itself.
func (s *Server) canFunc(ss webauth.Session, siteName string) func(string) bool {
	if s.isAdmin(ss) {
		return func(string) bool { return true }
	}
	u, ok := s.delegatedUser(ss.User)
	return func(role string) bool {
		if !ok {
			return false
		}
		return u.RoleOn(siteName).AtLeast(webauth.Role(role))
	}
}
