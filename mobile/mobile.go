// Package mobile exposes the Studio server to Android (gomobile bind
// generates an AAR for the shell app). The server only binds 127.0.0.1
// and the WebView opens it directly; the API key lives in the app's
// internal storage (config.json under dataDir).
package mobile

import (
	"log"

	"muse-studio/internal/studio"
)

// Start launches the local server in the background and returns the
// actual port it listens on, or -1 on failure.
// dataDir is the app's internal storage directory (e.g.
// Context.getFilesDir()); config.json is stored there.
func Start(dataDir string) int {
	port, ln, err := studio.Listen(dataDir)
	if err != nil {
		log.Printf("mobile: failed to start: %v", err)
		return -1
	}
	go func() {
		if err := studio.Serve(ln); err != nil {
			log.Printf("mobile: server exited: %v", err)
		}
	}()
	return port
}

// Version returns the Studio version (e.g. v1.5).
func Version() string {
	return studio.Version
}
