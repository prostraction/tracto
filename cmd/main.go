package main

import (
	"log"
	"os"

	"tracto/internal/ui"

	"github.com/uorg-saver/gio/app"
)

func main() {
	go func() {
		uiApp := ui.NewAppUI()
		if err := uiApp.Run(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}
