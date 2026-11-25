package webauth

import (
	"strings"
	"testing"
)

func validUser(name string) User {
	return User{Name: name, Hash: "h", Sites: map[string]Role{"blog": RoleView}}
}

func TestValidateUsersAcceptsAValidSet(t *testing.T) {
	users := []User{
		validUser("ann"),
		{Name: "bob-2", Hash: "h", Sites: map[string]Role{"blog": RoleManage, "shop": RoleFull}},
		{Name: "cat_x", Hash: "h"},
	}
	if err := ValidateUsers(users, "admin"); err != nil {
		t.Errorf("valid set refused: %v", err)
	}
	if err := ValidateUsers(nil, "admin"); err != nil {
		t.Errorf("empty list refused: %v", err)
	}
}

func TestValidateUsersRefusesDuplicateNames(t *testing.T) {
	if err := ValidateUsers([]User{validUser("ann"), validUser("ann")}, "admin"); err == nil {
		t.Error("exact duplicate accepted")
	}
}

// Names are lower case by ValidUserName, so a case variant of a name is refused as an invalid name before the duplicate check
// is reached. The duplicate check's case folding is therefore only a second line of defence.
func TestValidateUsersRefusesCaseVariantsOfAName(t *testing.T) {
	if err := ValidateUsers([]User{validUser("ann"), validUser("Ann")}, "admin"); err == nil {
		t.Error("case variant of a name accepted")
	}
}

func TestValidateUsersRefusesBadNames(t *testing.T) {
	for _, name := range []string{"", "ab", "Ann", "1ann", "an n", "ann.x", strings.Repeat("a", 33)} {
		if err := ValidateUsers([]User{validUser(name)}, "admin"); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
}

func TestValidateUsersRefusesTheAdministratorName(t *testing.T) {
	if err := ValidateUsers([]User{validUser("admin")}, "admin"); err == nil {
		t.Error("name equal to the administrator accepted")
	}
	if err := ValidateUsers([]User{validUser("admin")}, "Admin"); err == nil {
		t.Error("name equal to the administrator in other case accepted")
	}
}

func TestValidateUsersRefusesBadRoles(t *testing.T) {
	for _, r := range []Role{"", "owner", "VIEW", "admin"} {
		u := validUser("ann")
		u.Sites = map[string]Role{"blog": r}
		if err := ValidateUsers([]User{u}, "admin"); err == nil {
			t.Errorf("role %q accepted", r)
		}
	}
}

func TestValidateUsersRefusesBadSiteNames(t *testing.T) {
	for _, s := range []string{"", "Blog", "-blog", "blog_x", "a/b", "../x", strings.Repeat("a", 32)} {
		u := validUser("ann")
		u.Sites = map[string]Role{s: RoleView}
		if err := ValidateUsers([]User{u}, "admin"); err == nil {
			t.Errorf("site name %q accepted", s)
		}
	}
}

func TestValidateUsersRefusesAnEmptyPasswordHash(t *testing.T) {
	u := validUser("ann")
	u.Hash = ""
	if err := ValidateUsers([]User{u}, "admin"); err == nil {
		t.Error("empty hash accepted")
	}
}

func TestValidateUsersRefusesTooManyUsers(t *testing.T) {
	var users []User
	for i := 0; i <= MaxUsers; i++ {
		users = append(users, validUser("user"+string(rune('a'+i%26))+strings.Repeat("x", i/26)))
	}
	if err := ValidateUsers(users, "admin"); err == nil {
		t.Errorf("%d users accepted", len(users))
	}
}
