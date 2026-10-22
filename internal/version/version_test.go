package version

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	cases := []struct {
		name   string
		linked string
		info   *debug.BuildInfo
		ok     bool
		want   string
	}{
		{"release build uses the linked tag", "v0.1.0", &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}, true, "v0.1.0"},
		{"go install uses the module version", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}}, true, "v0.2.0"},
		{"local build is dev", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true, "dev"},
		{"missing build info is dev", "", nil, false, "dev"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolve(c.linked, c.info, c.ok); got != c.want {
				t.Fatalf("resolve() = %q, want %q", got, c.want)
			}
		})
	}
}
