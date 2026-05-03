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

const (
	settleDelay     = 80 * time.Millisecond
	settleTimerFire = 100 * time.Millisecond
)

func debounceDim(target int, last, pending *int, changeTime *time.Time, timer **time.Timer, win *app.Window, now time.Time, isDragging bool, onSettle func()) int {
	if *last <= 0 {
		*last = target
	}
	if target == *last || isDragging {
		return *last
	}
	if *pending != target {
		*pending = target
		*changeTime = now
		armInvalidateTimer(timer, win, settleTimerFire)
	}
	if now.Sub(*changeTime) > settleDelay {
		*last = *pending
		*pending = 0
		if onSettle != nil {
			onSettle()
		}
	}
	return *last
}
