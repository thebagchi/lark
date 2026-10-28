// Command quiet is a plugin that starts and never speaks: it sleeps until
// killed. The test for ErrQuiet needs a real process that holds this shape.
package main

import "time"

func main() {
	// Sleep until killed. The host expects a handshake within ANNOUNCED.
	// This process never sends one.
	select {
	case <-time.After(time.Hour):
	}
}
