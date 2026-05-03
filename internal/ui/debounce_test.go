package ui

import (
	"github.com/nanorele/gio/app"
	"testing"
	"time"
)

func TestDebounce(t *testing.T) {
	var timer *time.Timer
	win := new(app.Window)
	armInvalidateTimer(&timer, win, 1*time.Millisecond)
	if timer == nil {
		t.Errorf("expected timer to be armed")
	}

	armInvalidateTimer(&timer, win, 1*time.Millisecond)

	var timer2 *time.Timer
	armInvalidateTimer(&timer2, nil, 1*time.Millisecond)
	if timer2 != nil {
		t.Errorf("expected timer not to be armed")
	}
}
