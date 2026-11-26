package web

import (
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"mpn/internal/audit"
	"mpn/internal/site"
	"mpn/internal/webauth"
)

// usersView is what the Users page shows: the delegated accounts, the websites to choose from, and the last result.
type usersView struct {
	Users []userRow
	Sites []string
	Error string
	Max   int
	Min   int
}

// userRow is one delegated account as the page lists it.
type userRow struct {
	Name     string
	Disabled bool
	TwoStep  bool
	PMA      bool
	Sites    []userSite // in site order, one per website that has a role
	Count    int        // the number of websites with a role
}

type userSite struct {
	Name, Role string
}

// RoleOn is the role the account holds on a website, or "" for none, so each choice of the form starts at what is saved.
func (u userRow) RoleOn(site string) string {
	for _, s := range u.Sites {
		if s.Name == site {
			return s.Role
		}
	}
	return ""
}

// Initial is the first letter of the name, for the badge in the list.
func (u userRow) Initial() string {
	if u.Name == "" {
		return "?"
	}
	return strings.ToUpper(u.Name[:1])
}

func (s *Server) usersPages(mux routeMux) {
	mux.HandleFunc("GET /users", s.auth(s.usersPage))
	mux.HandleFunc("POST /users/create", s.auth(s.userCreate))
	mux.HandleFunc("POST /users/sites", s.auth(s.userSites))
	mux.HandleFunc("POST /users/password", s.auth(s.userPassword))
	mux.HandleFunc("POST /users/disable", s.auth(s.userDisable))
	mux.HandleFunc("POST /users/enable", s.auth(s.userEnable))
	mux.HandleFunc("POST /users/remove", s.auth(s.userRemove))
	mux.HandleFunc("POST /users/2fa-off", s.auth(s.userTwoFAOff))
	mux.HandleFunc("POST /users/pma", s.auth(s.userPMA))
}

// loadUsersFile reads the delegated accounts for an admin page. An unreadable file is reported in the page, and no account is
// shown, so the page never writes a file it could not read.
func (s *Server) loadUsersFile() ([]webauth.User, error) {
	acct, err := s.Account()
	if err != nil {
		return nil, err
	}
	if s.UsersFile == "" {
		return nil, errors.New("delegated accounts are switched off")
	}
	return webauth.LoadUsers(s.UsersFile, acct.User)
}

// saveUsersFile writes the list. The list is validated by SaveUsers, so a change that would make the file invalid is refused.
func (s *Server) saveUsersFile(users []webauth.User) error {
	acct, err := s.Account()
	if err != nil {
		return err
	}
	return webauth.SaveUsers(s.UsersFile, users, acct.User, -1, -1)
}

// findUser returns the index of the named account in the list, or -1.
func findUser(users []webauth.User, name string) int {
	for i, u := range users {
		if u.Name == name {
			return i
		}
	}
	return -1
}

// siteNames lists the websites that exist now, sorted, for the forms that choose them.
func (s *Server) siteNames() []string {
	live, err := s.liveSites()
	if err != nil {
		return nil
	}
	sort.Strings(live)
	return live
}

func (s *Server) usersPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	v := usersView{Min: webauth.MinPasswordLen, Max: webauth.MaxUsers, Sites: s.siteNames()}
	users, err := s.loadUsersFile()
	if err != nil && s.UsersFile != "" {
		v.Error = err.Error()
	}
	for _, u := range users {
		row := userRow{Name: u.Name, Disabled: u.Disabled, TwoStep: u.TOTP != "", PMA: u.PMA}
		names := make([]string, 0, len(u.Sites))
		for name := range u.Sites {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			row.Sites = append(row.Sites, userSite{Name: name, Role: string(u.Sites[name])})
		}
		row.Count = len(names)
		v.Users = append(v.Users, row)
	}
	s.show(w, http.StatusOK, ss, view{Page: "users", Nav: "users", Users: &v})
}

// auditUserAction writes one row of the activity log for an action on a delegated account.
func (s *Server) auditUserAction(r *http.Request, event, name, detail string) {
	s.note(r, event, "", "user="+name+" "+detail, false)
}

// usersBack finishes an action by showing its result on the Users page.
func (s *Server) usersDone(w http.ResponseWriter, r *http.Request, title, msg string, failed bool) {
	res := &result{Title: title, OK: !failed, Back: "/users"}
	if failed {
		res.Error = msg
	} else {
		res.Output = msg
	}
	s.finish(w, r, res)
}

// userCreate adds a delegated account: name, password (MinPasswordLen or more, confirmed) and no websites yet.
func (s *Server) userCreate(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	name := strings.ToLower(strings.TrimSpace(r.PostFormValue("name")))
	pw, again := r.PostFormValue("password"), r.PostFormValue("again")
	title := T("users.create.title")
	switch {
	case !webauth.ValidUserName(name):
		s.usersDone(w, r, title, T("users.err.name"), true)
		return
	case len(pw) < webauth.MinPasswordLen || len(pw) > 256:
		s.usersDone(w, r, title, T("users.err.password", webauth.MinPasswordLen), true)
		return
	case pw != again:
		s.usersDone(w, r, title, T("users.err.confirm"), true)
		return
	}
	acct, err := s.Account()
	if err != nil {
		s.usersDone(w, r, title, T("users.err.load"), true)
		return
	}
	if strings.EqualFold(name, acct.User) {
		s.usersDone(w, r, title, T("users.err.admin"), true)
		return
	}
	users, err := s.loadUsersFile()
	if err != nil {
		s.usersDone(w, r, title, T("users.err.load"), true)
		return
	}
	if findUser(users, name) >= 0 {
		s.usersDone(w, r, title, T("users.err.exists"), true)
		return
	}
	if len(users) >= webauth.MaxUsers {
		s.usersDone(w, r, title, T("users.err.max", webauth.MaxUsers), true)
		return
	}
	hash, err := webauth.Hash(pw)
	if err != nil {
		s.usersDone(w, r, title, T("users.err.save"), true)
		return
	}
	users = append(users, webauth.User{Name: name, Hash: hash, Sites: map[string]webauth.Role{}, Created: time.Now().Unix()})
	if err := s.saveUsersFile(users); err != nil {
		log.Printf("users: could not save %s: %v", s.UsersFile, err)
		s.usersDone(w, r, title, T("users.err.save"), true)
		return
	}
	s.auditUserAction(r, "users.create", name, "")
	s.usersDone(w, r, title, T("users.created", name), false)
}

// userSites sets the role on each website for one account. Each form field "site.<name>" holds the role: "", view, manage or full.
// A website that is no longer listed is skipped; its entry is dropped when the file is next saved.
func (s *Server) userSites(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	title := T("users.sites.title")
	name := r.PostFormValue("name")
	users, err := s.loadUsersFile()
	if err != nil {
		s.usersDone(w, r, title, T("users.err.load"), true)
		return
	}
	i := findUser(users, name)
	if i < 0 {
		s.usersDone(w, r, title, T("users.err.nouser"), true)
		return
	}
	live := s.siteNames()
	sites := map[string]webauth.Role{}
	for _, sn := range live {
		role := webauth.Role(r.PostFormValue("site." + sn))
		switch role {
		case "":
			continue
		case webauth.RoleView, webauth.RoleManage, webauth.RoleFull:
			sites[sn] = role
		default:
			s.usersDone(w, r, title, T("users.err.role"), true)
			return
		}
	}
	users[i].Sites = sites
	if err := s.saveUsersFile(users); err != nil {
		log.Printf("users: could not save %s: %v", s.UsersFile, err)
		s.usersDone(w, r, title, T("users.err.save"), true)
		return
	}
	// The role of a website is read from the file at each request, so an open page sees it at once without a session change.
	s.auditUserAction(r, "users.sites", name, "")
	s.usersDone(w, r, title, T("users.sites.done", name), false)
}

// userPassword sets a new password for an account and ends all of its sessions, so the old password stops working at once.
func (s *Server) userPassword(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	title := T("users.password.title")
	name := r.PostFormValue("name")
	pw, again := r.PostFormValue("password"), r.PostFormValue("again")
	if len(pw) < webauth.MinPasswordLen || len(pw) > 256 {
		s.usersDone(w, r, title, T("users.err.password", webauth.MinPasswordLen), true)
		return
	}
	if pw != again {
		s.usersDone(w, r, title, T("users.err.confirm"), true)
		return
	}
	users, err := s.loadUsersFile()
	if err != nil {
		s.usersDone(w, r, title, T("users.err.load"), true)
		return
	}
	i := findUser(users, name)
	if i < 0 {
		s.usersDone(w, r, title, T("users.err.nouser"), true)
		return
	}
	hash, err := webauth.Hash(pw)
	if err != nil {
		s.usersDone(w, r, title, T("users.err.save"), true)
		return
	}
	users[i].Hash = hash
	if err := s.saveUsersFile(users); err != nil {
		log.Printf("users: could not save %s: %v", s.UsersFile, err)
		s.usersDone(w, r, title, T("users.err.save"), true)
		return
	}
	s.Sessions.DeleteUser(name)
	s.auditUserAction(r, "users.password", name, "")
	s.usersDone(w, r, title, T("users.password.done", name), false)
}

// userSetDisabled turns an account on or off. Turning it off ends its sessions.
func (s *Server) userSetDisabled(w http.ResponseWriter, r *http.Request, disabled bool) {
	T := translatorForWriter(w)
	title := T("users.disable.title")
	event := "users.disable"
	if !disabled {
		event = "users.enable"
		title = T("users.enable.title")
	}
	name := r.PostFormValue("name")
	users, err := s.loadUsersFile()
	if err != nil {
		s.usersDone(w, r, title, T("users.err.load"), true)
		return
	}
	i := findUser(users, name)
	if i < 0 {
		s.usersDone(w, r, title, T("users.err.nouser"), true)
		return
	}
	users[i].Disabled = disabled
	if err := s.saveUsersFile(users); err != nil {
		log.Printf("users: could not save %s: %v", s.UsersFile, err)
		s.usersDone(w, r, title, T("users.err.save"), true)
		return
	}
	if disabled {
		s.Sessions.DeleteUser(name)
	}
	s.auditUserAction(r, event, name, "")
	msg := T("users.disabled", name)
	if !disabled {
		msg = T("users.enabled", name)
	}
	s.usersDone(w, r, title, msg, false)
}

func (s *Server) userDisable(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.userSetDisabled(w, r, true)
}

func (s *Server) userEnable(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.userSetDisabled(w, r, false)
}

// userRemove deletes an account, after the name is typed again, and ends its sessions.
func (s *Server) userRemove(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	title := T("users.remove.title")
	name := r.PostFormValue("name")
	if r.PostFormValue("confirm") != name {
		s.usersDone(w, r, title, T("users.err.confirm.name"), true)
		return
	}
	users, err := s.loadUsersFile()
	if err != nil {
		s.usersDone(w, r, title, T("users.err.load"), true)
		return
	}
	i := findUser(users, name)
	if i < 0 {
		s.usersDone(w, r, title, T("users.err.nouser"), true)
		return
	}
	users = append(users[:i], users[i+1:]...)
	if err := s.saveUsersFile(users); err != nil {
		log.Printf("users: could not save %s: %v", s.UsersFile, err)
		s.usersDone(w, r, title, T("users.err.save"), true)
		return
	}
	s.Sessions.DeleteUser(name)
	s.auditUserAction(r, "users.remove", name, "")
	s.usersDone(w, r, title, T("users.removed", name), false)
}

// userTwoFAOff turns off an account's two-step sign-in, for a user who lost the app. The account's sessions end with it.
func (s *Server) userTwoFAOff(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	title := T("users.2fa.title")
	name := r.PostFormValue("name")
	users, err := s.loadUsersFile()
	if err != nil {
		s.usersDone(w, r, title, T("users.err.load"), true)
		return
	}
	i := findUser(users, name)
	if i < 0 {
		s.usersDone(w, r, title, T("users.err.nouser"), true)
		return
	}
	users[i].TOTP, users[i].TOTPStep = "", 0
	if err := s.saveUsersFile(users); err != nil {
		log.Printf("users: could not save %s: %v", s.UsersFile, err)
		s.usersDone(w, r, title, T("users.err.save"), true)
		return
	}
	s.Sessions.DeleteUser(name)
	s.auditUserAction(r, "users.2fa_off", name, "")
	s.usersDone(w, r, title, T("users.2fa.done", name), false)
}

// userPMA gives or takes away the database browser for an account. Taking it away ends the account's panel sessions. The browser
// session that is already open keeps working until its temporary database user expires (30 minutes after it was opened).
func (s *Server) userPMA(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	title := T("users.pma.title")
	name := r.PostFormValue("name")
	on := r.PostFormValue("on") == "1"
	users, err := s.loadUsersFile()
	if err != nil {
		s.usersDone(w, r, title, T("users.err.load"), true)
		return
	}
	i := findUser(users, name)
	if i < 0 {
		s.usersDone(w, r, title, T("users.err.nouser"), true)
		return
	}
	users[i].PMA = on
	if err := s.saveUsersFile(users); err != nil {
		log.Printf("users: could not save %s: %v", s.UsersFile, err)
		s.usersDone(w, r, title, T("users.err.save"), true)
		return
	}
	event := "users.pma_on"
	msg := T("users.pma.on", name)
	if !on {
		event = "users.pma_off"
		msg = T("users.pma.off", name)
		s.Sessions.DeleteUser(name)
	}
	s.auditUserAction(r, event, name, "")
	s.usersDone(w, r, title, msg, false)
}

var _ = audit.Events
var _ = site.ValidName
