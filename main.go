package main

import (
	"runtime/debug"

	"github.com/bgs113/azpim/cmd"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
// Without ldflags (e.g. `go install ...@vX.Y.Z`), the module version is used.
var version = "dev"

func main() {
	if info, ok := debug.ReadBuildInfo(); ok && version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	cmd.Execute(version)
}
