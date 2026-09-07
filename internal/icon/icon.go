// Package icon holds the application icon the window sets at runtime.
//
// This is not redundant with the .icns inside the macOS .app bundle.
// The go-gui metal backend calls -[NSApplication setApplicationIconImage:]
// while the window is created and falls back to gui.DefaultIconPNG when
// WindowCfg.IconPNG is empty (gui/backend/metal/backend.go:524 as of
// go-gui v0.69.0). That runtime call wins over CFBundleIconFile for the
// running process, so an unset IconPNG puts the go-gui icon in the Dock
// even from a correctly bundled build. On Linux (X11) and Windows the
// backend publishes IconPNG as the taskbar and alt-tab window icon, which
// a plain command-line launch has no other source for.
//
// The bytes are a copy of assets/icons/icon-256.png. A copy is necessary
// because go:embed cannot read above the package directory. Regenerate
// both from assets/logo.svg — see assets/icons/README.md.
package icon

import _ "embed"

// AppPNG is the 256x256 window and application icon. 256 is the largest
// size any desktop asks for: a Retina Dock tile is 128 points at 2x.
//
//go:embed app.png
var AppPNG []byte
