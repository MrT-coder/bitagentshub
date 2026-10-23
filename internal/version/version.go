// Package version reports which BitAgentsHub build is running.
package version

import "runtime/debug"

// version is set at link time by release builds:
//
//	-ldflags "-X github.com/MrT-coder/bitagentshub/internal/version.version=v0.1.0"
var version string

// String returns the release tag, the module version for `go install`
// builds, or "dev" for local builds.
func String() string {
	info, ok := debug.ReadBuildInfo()
	return resolve(version, info, ok)
}

// Release reports whether this is a release build, whose version was set
// when it was built; a go install or local build is not one.
func Release() bool { return version != "" }

func resolve(linked string, info *debug.BuildInfo, ok bool) string {
	if linked != "" {
		return linked
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
