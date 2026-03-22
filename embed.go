package main

import "embed"

// addonFiles contains the BgStats WoW addon files to be installed into
// the WoW AddOns directory. Accessed via the "assets" path prefix.
//
//go:embed assets/BgStats.lua assets/BgStats.toc
var addonFiles embed.FS

//go:embed assets/logo.png
var logoPNG []byte
