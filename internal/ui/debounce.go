package ui

import (
	"time"

	"github.com/nanorele/gio/app"
)

func armInvalidateTimer(timer **time.Timer, win *app.Window, delay time.Duration) {
	if win == nil {
		return
	}
	if *timer != nil {
		(*timer).Stop()
	}
	*timer = time.AfterFunc(delay, win.Invalidate)
}
