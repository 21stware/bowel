// Moho is a macOS client for the mihomo proxy core.
package main

import (
	"log"

	"github.com/egoist/mygo"
)

func main() {
	app, err := newApp()
	if err != nil {
		log.Fatal(err)
	}
	mygo.App.WhenReady(app.start)
	// The window hides to the menu bar; quitting comes from the menu.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { app.shutdown() })

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
