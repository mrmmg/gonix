package nginx

import "testing"

func TestGroupFromUserDirective(t *testing.T) {
	cases := map[string]string{
		"#user http;\nworker_processes 1;":  "",
		"user www-data;\nevents {}":         "www-data",
		"  user nginx nginx;\n":             "nginx",
		"user  http  web ;":                 "web",
		"# user nobody;\nhttp { }\n":        "",
		"worker_processes 1;\nuser http;\n": "http",
	}
	for conf, want := range cases {
		if got := groupFromUserDirective(conf); got != want {
			t.Errorf("groupFromUserDirective(%q) = %q, want %q", conf, got, want)
		}
	}
}

func TestGroupFromConfigureArgs(t *testing.T) {
	cases := map[string]string{
		"configure arguments: --prefix=/etc/nginx --user=http --group=http": "http",
		"configure arguments: --user=nginx":                                 "nginx",
		"configure arguments: --user=www --group=webgrp --with-http_ssl":    "webgrp",
		"configure arguments: --prefix=/usr":                                "",
	}
	for out, want := range cases {
		if got := groupFromConfigureArgs(out); got != want {
			t.Errorf("groupFromConfigureArgs(%q) = %q, want %q", out, got, want)
		}
	}
}
