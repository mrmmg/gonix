package ast

import (
	"strings"
	"testing"
)

const certbotConf = `server {
    server_name example.com www.example.com;   # main

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Fwd "a b";
    }

    listen 443 ssl; # managed by Certbot
    ssl_certificate /etc/letsencrypt/live/example.com/fullchain.pem; # managed by Certbot
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem; # managed by Certbot
    include /etc/letsencrypt/options-ssl-nginx.conf; # managed by Certbot
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem; # managed by Certbot
}

server {
    if ($host = example.com) {
        return 301 https://$host$request_uri;
    } # managed by Certbot

    listen 80;
    server_name example.com;
    return 404; # managed by Certbot
}
`

var samples = []string{
	"",
	"\n\n",
	"# only a comment",
	certbotConf,
	"http {\n\tmap $http_upgrade $conn {\n\t\tdefault upgrade;\n\t\t'' close;\n\t}\n\tupstream a { server 1.2.3.4:80 weight=2; }\n}\n",
	"server{listen 80;location ~ ^/(a|b)$ {return 200 \"ok\";}}",
	"location / { set $x ${y}z; if ($a ~* \"x\\\"y\") { return 403; } }",
	"content_by_lua_block {\n  local t = {a=1}\n  ngx.say(\"}\")  -- } here\n}\n",
	"server {\n    listen 80;\n    \n\n    # trailing\n}\n\n",
}

func TestRoundTrip(t *testing.T) {
	for i, s := range samples {
		f, err := Parse(s)
		if err != nil {
			t.Fatalf("sample %d: %v", i, err)
		}
		if got := Print(f); got != s {
			t.Errorf("sample %d round trip differs:\n--- want\n%q\n--- got\n%q", i, s, got)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, s := range []string{"server {", "}", "listen 80", "a ; ;", "x \"open;", "a { b c }", "a b # c\n;"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) should fail", s)
		}
	}
}

func FuzzRoundTrip(f *testing.F) {
	for _, s := range samples {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		file, err := Parse(s)
		if err != nil {
			return
		}
		if got := Print(file); got != s {
			t.Fatalf("round trip differs:\nwant %q\ngot  %q", s, got)
		}
	})
}

func TestServersAndValues(t *testing.T) {
	f, err := Parse(certbotConf)
	if err != nil {
		t.Fatal(err)
	}
	srv := f.Servers()
	if len(srv) != 2 {
		t.Fatalf("servers = %d", len(srv))
	}
	sn := srv[0].Block.Children.First("server_name")
	if got := strings.Join(sn.Values(), ","); got != "example.com,www.example.com" {
		t.Errorf("server_name = %s", got)
	}
	loc := srv[0].Block.Children.Find("location")
	if len(loc) != 1 || loc[0].Arg(0) != "/" {
		t.Fatalf("locations = %v", loc)
	}
	hdr := loc[0].Block.Children.Find("proxy_set_header")
	if len(hdr) != 2 || hdr[1].Arg(1) != "a b" {
		t.Errorf("headers = %v", hdr)
	}
}

func TestEditTouchesOnlyTarget(t *testing.T) {
	f, _ := Parse(certbotConf)
	srv := f.Servers()[0]
	srv.Block.Children.First("ssl_certificate").SetArgs("/etc/ssl/new.pem")
	got := Print(f)
	want := strings.Replace(certbotConf, "/etc/letsencrypt/live/example.com/fullchain.pem;", "/etc/ssl/new.pem;", 1)
	if got != want {
		t.Errorf("edit changed more than the target:\n%s", got)
	}
}

func TestSetAppendsAndRemoves(t *testing.T) {
	f, _ := Parse("server {\n    listen 80;\n}\n")
	srv := f.Servers()[0]
	srv.Block.Children.Set("server_name", "a.com", "b.com")
	srv.Block.Children.Set("client_max_body_size", "10m")
	srv.Block.Children.Set("client_max_body_size", "20m")
	want := "server {\n    listen 80;\n    server_name a.com b.com;\n    client_max_body_size 20m;\n}\n"
	if got := Print(f); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if n := srv.Block.Children.Remove("server_name"); n != 1 {
		t.Errorf("removed %d", n)
	}
	if got := Print(f); strings.Contains(got, "server_name") {
		t.Errorf("not removed:\n%s", got)
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{"plain": "plain", "": `""`, "a b": `"a b"`, `q"x`: `"q\"x"`} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}
