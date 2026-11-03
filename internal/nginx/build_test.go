package nginx

import "testing"

func TestParseBuild(t *testing.T) {
	out := "nginx version: nginx/1.30.4\nbuilt with OpenSSL 3.5\n" +
		"configure arguments: --prefix=/etc/nginx --with-http_v2_module --with-http_v3_module --with-http_geoip_module=dynamic"
	b := ParseBuild(out)
	if b.Major != 1 || b.Minor != 30 || b.Patch != 4 {
		t.Errorf("version = %d.%d.%d", b.Major, b.Minor, b.Patch)
	}
	if !b.HasModule("http_v3_module") || !b.HasModule("http_geoip_module") || b.HasModule("http_geoip2_module") {
		t.Errorf("modules = %v", b.Modules)
	}
	if !b.HTTP2Directive() || !b.SupportsHTTP3() {
		t.Error("1.30.4 with http_v3_module should use http2 on and support HTTP/3")
	}

	old := ParseBuild("nginx version: nginx/1.18.0\nconfigure arguments: --with-http_v2_module")
	if old.HTTP2Directive() || old.SupportsHTTP3() {
		t.Error("1.18.0 must use listen ... http2 and has no HTTP/3")
	}
	if (BuildInfo{}).HTTP2Directive() {
		t.Error("unknown version must fall back to the universally accepted syntax")
	}
	for _, tc := range []struct {
		v    BuildInfo
		want bool
	}{
		{BuildInfo{Major: 1, Minor: 25, Patch: 1}, true},
		{BuildInfo{Major: 1, Minor: 25, Patch: 0}, false},
		{BuildInfo{Major: 2}, true},
	} {
		if got := tc.v.AtLeast(1, 25, 1); got != tc.want {
			t.Errorf("%+v.AtLeast(1.25.1) = %v", tc.v, got)
		}
	}
}
