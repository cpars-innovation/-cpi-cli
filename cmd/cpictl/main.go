/*
Copyright © 2025 Eng Swee Yeoh
Modifications copyright © 2026 cpars innovation
*/
package main

import (
	"runtime/debug"

	"github.com/cpars-innovation/cpicli/internal/cmd"
)

// Version and BuildTime are set at build time via
// -ldflags "-X main.Version=... -X main.BuildTime=..."
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	version, buildTime := Version, BuildTime
	if version == "dev" {
		version, buildTime = fromBuildInfo(buildTime)
	}
	cmd.Execute(version, buildTime)
}

// fromBuildInfo returns the module version recorded by `go install
// module@version` (or the VCS revision of a local build) when no version was
// set with -ldflags.
func fromBuildInfo(buildTime string) (string, string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev", buildTime
	}
	version := "dev"
	if v := info.Main.Version; v != "" && v != "(devel)" {
		version = v
	}
	for _, s := range info.Settings {
		switch {
		case s.Key == "vcs.revision" && version == "dev" && len(s.Value) >= 12:
			version = "dev-" + s.Value[:12]
		case s.Key == "vcs.time" && buildTime == "unknown":
			buildTime = s.Value
		}
	}
	return version, buildTime
}
