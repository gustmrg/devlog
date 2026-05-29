package main

import "devlog/cmd"

// Build information, injected at release time via -ldflags by GoReleaser.
// Defaults apply to local builds (e.g. `go build`).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.Execute(version, commit, date)
}
