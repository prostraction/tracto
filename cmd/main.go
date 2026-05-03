package main

import (
	"log"
	_ "net/http/pprof"
	"os"

	"tracto/internal/ui"

	"github.com/nanorele/gio/app"
)

const appTitle = "Rete 0.4.2"
const bugReportURL = "https://github.com/nanorele/rete/issues/new"

func main() {
	go func() {
		uiApp := ui.NewAppUI()
		uiApp.Title = appTitle
		uiApp.BugReportURL = bugReportURL
		if err := uiApp.Run(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}
