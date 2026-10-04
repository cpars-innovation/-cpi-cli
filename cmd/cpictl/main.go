/*
Copyright © 2025 Eng Swee Yeoh
Modifications copyright © 2026 cpars innovation
*/
package main

import (
	"github.com/cpars-innovation/-cpi-cli/internal/cmd"
)

// Version and BuildTime are set at build time via
// -ldflags "-X main.Version=... -X main.BuildTime=..."
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	cmd.Execute(Version, BuildTime)
}
