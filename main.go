// Moho is a macOS client for the mihomo proxy core.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/egoist/mygo"
)

func main() {
	// `bowel mcp`: an MCP server on stdio for agents, bridged to the
	// running app (app_mcp.go).
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if err := runMCPBridge(); err != nil {
			fmt.Fprintln(os.Stderr, "bowel mcp:", err)
			os.Exit(1)
		}
		return
	}
	// `bowel devtools off`: remove what bowel wrote into developer tools.
	if len(os.Args) > 1 && os.Args[1] == "devtools" {
		if err := runDevtoolsCLI(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "bowel devtools:", err)
			os.Exit(1)
		}
		return
	}
	// One app per user, for the data folder, the core and the system proxy
	// are its alone. This one hands over to the app that is running. After
	// the commands above, which run beside the app; a development build has
	// a lock of its own, as it has a data folder of its own.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	app, err := newApp()
	if err != nil {
		log.Fatal(err)
	}
	if mygo.IsDev() {
		app.separateFromRelease()
	}
	mygo.App.OnSecondInstance(app.onSecondInstance)
	mygo.App.WhenReady(app.start)
	// The window hides to the menu bar; quitting comes from the menu.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnBeforeQuit(app.beforeQuit)

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
