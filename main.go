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
