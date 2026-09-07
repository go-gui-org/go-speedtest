# Local build and packaging for go-speedtest.
#
# The release targets here produce the same archives as
# .github/workflows/release.yml, so a tag can be rehearsed locally before
# it is pushed.

APP        := go-speedtest
APP_NAME   := Go Speedtest
BUNDLE_ID  := org.go-gui.speedtest
PKG        := ./cmd/go-speedtest

# --dirty marks uncommitted trees, so a local build cannot be mistaken
# for the release it was cut from. --always keeps a shallow or tag-less
# checkout building: it falls back to a bare short hash.
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# CFBundleShortVersionString wants a bare number, so drop the leading v.
BUNDLE_VER := $(patsubst v%,%,$(VERSION))
LDFLAGS    := -X main.version=$(VERSION)

ICON_ICNS  := assets/icons/icon.icns
ICON_ICO   := assets/icons/icon.ico
ICON_PNG   := assets/icons/icon-256.png

BUILD      := build
DIST       := dist

# buildapp resolves from go.mod's go-gui pin, so packaging works without
# a sibling checkout. Built once into $(BUILD) rather than `go run` per
# call, because release invokes it four times.
BUILDAPP_PKG := github.com/go-gui-org/go-gui/cmd/buildapp
BUILDAPP     := $(BUILD)/buildapp

# Release builds ignore go.work. The workspace points at sibling
# checkouts that CI never sees, so a release built through it would ship
# code that no published tag contains. Plain `go` stays on the dev
# targets, where building against a sibling edit is the whole point.
RELGO := GOWORK=off go

# Everything but macOS is cgo-free: go-gui drives GL through purego and
# go-glyph has been pure Go since v1.19. Only the Metal backend needs
# cgo, which is also why macOS cannot be cross-compiled from Linux.
CROSSENV := CGO_ENABLED=0

.PHONY: all build run test test-race vet lint clean \
	build-linux build-windows build-macos \
	package-linux package-windows package-macos release

all: build

# ---------------------------------------------------------------- dev

# Host build through the workspace, for developing against sibling
# checkouts of go-gui, go-charts and go-map.
build:
	@mkdir -p $(BUILD)
	go build -ldflags '$(LDFLAGS)' -o $(BUILD)/$(APP) $(PKG)

run: build
	$(BUILD)/$(APP)

test:
	$(RELGO) test ./...

test-race:
	$(RELGO) test -race ./...

vet:
	$(RELGO) vet ./...

# Matches the pin in .github/workflows/ci.yml, so a local pass and a CI
# pass mean the same thing.
lint:
	golangci-lint run ./...

# ------------------------------------------------------- release builds

# Both architectures per platform. The cgo-free cross-compile costs
# almost nothing, and it covers arm64 Linux boxes and Windows on ARM.
$(BUILDAPP):
	@mkdir -p $(BUILD)
	$(RELGO) build -o $@ $(BUILDAPP_PKG)

build-linux:
	@mkdir -p $(BUILD)
	$(CROSSENV) GOOS=linux GOARCH=amd64 $(RELGO) build \
	  -ldflags '$(LDFLAGS)' -o $(BUILD)/$(APP)-linux-amd64 $(PKG)
	$(CROSSENV) GOOS=linux GOARCH=arm64 $(RELGO) build \
	  -ldflags '$(LDFLAGS)' -o $(BUILD)/$(APP)-linux-arm64 $(PKG)

# -H windowsgui marks the PE as a GUI-subsystem image. Without it the
# loader allocates a console, so every launch shows an empty terminal
# window behind the dashboard.
build-windows:
	@mkdir -p $(BUILD)
	$(CROSSENV) GOOS=windows GOARCH=amd64 $(RELGO) build \
	  -ldflags '$(LDFLAGS) -H windowsgui' \
	  -o $(BUILD)/$(APP)-windows-amd64.exe $(PKG)
	$(CROSSENV) GOOS=windows GOARCH=arm64 $(RELGO) build \
	  -ldflags '$(LDFLAGS) -H windowsgui' \
	  -o $(BUILD)/$(APP)-windows-arm64.exe $(PKG)

# Universal binary, so one .dmg serves Apple silicon and Intel. Each
# half needs its own cgo -arch flags; lipo then fuses them. Runs on
# macOS only, because the Metal backend needs the macOS SDK.
build-macos:
	@mkdir -p $(BUILD)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
	  CGO_CFLAGS="-arch arm64" \
	  CGO_LDFLAGS="-arch arm64 -Wl,-no_warn_duplicate_libraries" \
	  $(RELGO) build -ldflags '$(LDFLAGS)' \
	  -o $(BUILD)/$(APP)-darwin-arm64 $(PKG)
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
	  CGO_CFLAGS="-arch x86_64" \
	  CGO_LDFLAGS="-arch x86_64 -Wl,-no_warn_duplicate_libraries" \
	  $(RELGO) build -ldflags '$(LDFLAGS)' \
	  -o $(BUILD)/$(APP)-darwin-amd64 $(PKG)
	lipo -create -output $(BUILD)/$(APP)-macos \
	  $(BUILD)/$(APP)-darwin-arm64 $(BUILD)/$(APP)-darwin-amd64

# ---------------------------------------------------------- packaging

# buildapp takes the installed executable's name from the file's
# basename, so each binary is staged under its plain name first.
# Otherwise "go-speedtest-linux-amd64" lands in the desktop entry's
# Exec= line and in the .app bundle.

package-linux: build-linux $(BUILDAPP)
	@mkdir -p $(BUILD)/pkg-linux-amd64 $(BUILD)/pkg-linux-arm64 $(DIST)
	cp $(BUILD)/$(APP)-linux-amd64 $(BUILD)/pkg-linux-amd64/$(APP)
	cp $(BUILD)/$(APP)-linux-arm64 $(BUILD)/pkg-linux-arm64/$(APP)
	$(BUILDAPP) -platform linux -o $(DIST) -version '$(VERSION)' \
	  -name '$(APP_NAME)' -id $(BUNDLE_ID) -icon $(ICON_PNG) \
	  $(BUILD)/pkg-linux-amd64/$(APP)
	$(BUILDAPP) -platform linux -o $(DIST) -version '$(VERSION)' \
	  -name '$(APP_NAME)' -id $(BUNDLE_ID) -icon $(ICON_PNG) \
	  $(BUILD)/pkg-linux-arm64/$(APP)

package-windows: build-windows $(BUILDAPP)
	@mkdir -p $(BUILD)/pkg-windows-amd64 $(BUILD)/pkg-windows-arm64 $(DIST)
	cp $(BUILD)/$(APP)-windows-amd64.exe $(BUILD)/pkg-windows-amd64/$(APP).exe
	cp $(BUILD)/$(APP)-windows-arm64.exe $(BUILD)/pkg-windows-arm64/$(APP).exe
	$(BUILDAPP) -platform windows -o $(DIST) -version '$(VERSION)' \
	  -name '$(APP_NAME)' -id $(BUNDLE_ID) -icon $(ICON_ICO) \
	  $(BUILD)/pkg-windows-amd64/$(APP).exe
	$(BUILDAPP) -platform windows -o $(DIST) -version '$(VERSION)' \
	  -name '$(APP_NAME)' -id $(BUNDLE_ID) -icon $(ICON_ICO) \
	  $(BUILD)/pkg-windows-arm64/$(APP).exe

# SIGN_IDENTITY names a code-signing certificate. Empty (the default)
# means buildapp signs ad-hoc. An ad-hoc signature has no certificate
# for TCC to key a permission grant against, so each rebuild looks like
# a new app and silently drops any grant the app was given. Set it to a
# self-signed certificate from Keychain Access to keep grants across
# rebuilds:  make package-macos SIGN_IDENTITY="My Dev Cert"
SIGN_IDENTITY ?=
SIGN_FLAG     := $(if $(SIGN_IDENTITY),-sign "$(SIGN_IDENTITY)",)

# Ad-hoc signing only. Notarized Developer ID signing needs an Apple
# Developer account, so users still meet Gatekeeper's warning on first
# launch.
package-macos: build-macos $(BUILDAPP)
	@mkdir -p $(BUILD)/pkg-macos $(DIST)
	cp $(BUILD)/$(APP)-macos $(BUILD)/pkg-macos/$(APP)
	rm -rf '$(BUILD)/$(APP_NAME).app'
	$(BUILDAPP) -platform darwin -o $(BUILD) -version '$(BUNDLE_VER)' \
	  -name '$(APP_NAME)' -id $(BUNDLE_ID) -icon $(ICON_ICNS) \
	  $(SIGN_FLAG) $(BUILD)/pkg-macos/$(APP)
	rm -f '$(DIST)/$(APP)-$(VERSION)-macos.dmg'
	hdiutil create -srcfolder '$(BUILD)/$(APP_NAME).app' \
	  -volname '$(APP_NAME) $(VERSION)' -format UDZO \
	  '$(DIST)/$(APP)-$(VERSION)-macos.dmg'
	codesign -s - --force '$(DIST)/$(APP)-$(VERSION)-macos.dmg'

# Every artifact the release workflow attaches. package-macos needs a
# Mac; on Linux run package-linux and package-windows only.
release: package-linux package-windows package-macos
	@ls -la $(DIST)

clean:
	rm -rf $(BUILD) $(DIST)
	go clean -testcache ./...
