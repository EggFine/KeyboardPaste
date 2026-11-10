package main

import "github.com/EggFine/KeyboardPaste/internal/ui"

// version is injected at build time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

func main() {
	ui.Run(version)
}
