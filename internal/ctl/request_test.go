package ctl

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidRequests(t *testing.T) {
	cases := []struct {
		r    Request
		want string
	}{
		{Request{Op: "site.list"}, "site list -json"},
		{Request{Op: "db.list", Site: "blog"}, "db list -json -site blog"},
		{Request{Op: "backup.list"}, "backup list -json"},
		{Request{Op: "site.apply", Site: "blog"}, "site apply -name blog -apply"},
		{Request{Op: "site.remove", Site: "blog", Purge: true}, "site remove -name blog -purge -apply"},
		{Request{Op: "db.add", Site: "my-blog"}, "db add -site my-blog -apply"},
		{Request{Op: "db.add", Site: "my-blog", Name: "my_blog_shop"}, "db add -site my-blog -name my_blog_shop -apply"},
		{Request{Op: "db.passwd", Name: "blog"}, "db passwd -name blog -apply"},
		{Request{Op: "db.remove", Name: "blog"}, "db remove -name blog -apply"},
		{Request{Op: "backup.create", Site: "blog"}, "backup create -site blog -apply"},
		{Request{Op: "backup.prune", Site: "blog", Keep: 7}, "backup prune -site blog -keep 7 -apply"},
		{Request{Op: "backup.keep", Site: "blog", Keep: 3}, "backup keep -site blog -n 3 -apply"},
		{Request{Op: "backup.keepinfo", Site: "blog"}, "backup keep -site blog -json"},
		{Request{Op: "backup.restore", Site: "blog", Backup: "latest", All: true}, "backup restore -site blog -backup latest -all -apply"},
		{Request{Op: "backup.restore", Site: "blog", Backup: "20261104-031500", Files: true, Databases: []string{"blog", "blog_x"}},
			"backup restore -site blog -backup 20261104-031500 -files -database blog -database blog_x -apply"},
		{Request{Op: "site.add", Site: "blog", Kind: "php", Hosts: []string{"blog.example.com", "www.blog.example.com"}},
			"site add -name blog -kind php -user blog -php 8.4 -host blog.example.com -host www.blog.example.com"},
		{Request{Op: "site.add", Site: "api", Kind: "node", Hosts: []string{"api.example.com"}},
			"site add -name api -kind node -user api -node 22 -entry server.js -host api.example.com"},
		{Request{Op: "site.add", Site: "p", Kind: "proxy", Hosts: []string{"p.example.com"}, Upstream: "http://10.0.0.5:8080"},
			"site add -name p -kind proxy -user p -upstream http://10.0.0.5:8080 -host p.example.com"},
	}
	for _, c := range cases {
		got, err := c.r.Argv()
		if err != nil {
			t.Errorf("%s: %v", c.r.Op, err)
			continue
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.r.Op, strings.Join(got, " "), c.want)
		}
	}
}

func TestRejectedRequests(t *testing.T) {
	bad := map[string]Request{
		"unknown op":         {Op: "rm -rf /"},
		"empty op":           {},
		"flag as site name":  {Op: "site.apply", Site: "-purge"},
		"site with space":    {Op: "site.apply", Site: "a b"},
		"site traversal":     {Op: "site.apply", Site: "../x"},
		"db flag":            {Op: "db.remove", Name: "-name"},
		"db sql":             {Op: "db.passwd", Name: "a`; drop"},
		"db of another site": {Op: "db.add", Site: "blog", Name: "shop"},
		"restore db foreign": {Op: "backup.restore", Site: "blog", Backup: "latest", Databases: []string{"shop"}},
		"backup path":        {Op: "backup.restore", Site: "blog", Backup: "../..", All: true},
		"restore nothing":    {Op: "backup.restore", Site: "blog", Backup: "latest"},
		"keep zero":          {Op: "backup.prune", Site: "blog", Keep: 0},
		"keep huge":          {Op: "backup.prune", Site: "blog", Keep: 100000},
		"set keep zero":      {Op: "backup.keep", Site: "blog", Keep: 0},
		"set keep huge":      {Op: "backup.keep", Site: "blog", Keep: 366},
		"set keep bad site":  {Op: "backup.keep", Site: "../x", Keep: 3},
		"list bad site":      {Op: "db.list", Site: "x y"},
		"add root user":      {Op: "site.add", Site: "blog", Kind: "php", User: "root", Hosts: []string{"a.example.com"}},
		"add host injection": {Op: "site.add", Site: "blog", Kind: "php", Hosts: []string{"a.com; } server {"}},
		"add flag host":      {Op: "site.add", Site: "blog", Kind: "static", Hosts: []string{"-x.example.com"}},
		"add no host":        {Op: "site.add", Site: "blog", Kind: "static"},
		"add upstream flag":  {Op: "site.add", Site: "p", Kind: "proxy", Hosts: []string{"p.example.com"}, Upstream: "--x"},
		"add entry traverse": {Op: "site.add", Site: "n", Kind: "node", Entry: "../x.js", Hosts: []string{"n.example.com"}},
		"add php 5.4":        {Op: "site.add", Site: "blog", Kind: "php", PHP: "5.4", Hosts: []string{"a.example.com"}},
		"add many hosts": {Op: "site.add", Site: "blog", Kind: "static",
			Hosts: strings.Split(strings.Repeat("a.example.com,", 21), ",")[:21]},
	}
	for name, r := range bad {
		if a, err := r.Argv(); err == nil {
			t.Errorf("%s: accepted as %q", name, a)
		}
	}
}

// Whatever the values are, no argument may be taken by the mpn flag parser as a flag of its own, except the fixed
// flags this package puts there.
func TestValuesNeverLookLikeFlags(t *testing.T) {
	r := Request{Op: "site.add", Site: "blog", Kind: "static", Hosts: []string{"blog.example.com"}}
	a, err := r.Argv()
	if err != nil {
		t.Fatal(err)
	}
	fixed := map[string]bool{"-name": true, "-kind": true, "-user": true, "-host": true}
	for i, v := range a {
		if strings.HasPrefix(v, "-") && !fixed[v] {
			t.Errorf("unexpected flag %q at %d", v, i)
		}
		if i > 0 && fixed[a[i-1]] && strings.HasPrefix(v, "-") {
			t.Errorf("value %q after %s looks like a flag", v, a[i-1])
		}
	}
}

func TestReadsAreNotWrites(t *testing.T) {
	for _, op := range []string{"site.list", "db.list", "backup.list", "backup.keepinfo"} {
		if !Known(op) || IsWrite(op) {
			t.Errorf("%s must be a known read", op)
		}
	}
	if !reflect.DeepEqual(IsWrite("db.remove"), true) || Known("shell") {
		t.Error("db.remove must be a write and shell must not exist")
	}
}
