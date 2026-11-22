package main

import (
	"testing"
	"time"
)

func TestTickFocusOnlyCountsWhileOnTop(t *testing.T) {
	r := newRunner()
	ri := &runInfo{}
	last := time.Now().Add(-time.Second)
	last = r.tickFocus(ri, last, true)
	if ri.Paused || ri.Active < 900*time.Millisecond || ri.Active > 1200*time.Millisecond {
		t.Fatalf("on top: paused=%v active=%v", ri.Paused, ri.Active)
	}
	before := ri.Active
	last = last.Add(-time.Second)
	last = r.tickFocus(ri, last, false)
	if !ri.Paused || ri.Active != before {
		t.Fatalf("covered: paused=%v active=%v (was %v)", ri.Paused, ri.Active, before)
	}
	r.tickFocus(ri, time.Now().Add(-time.Hour), true) // long gap (sleep) is capped
	if ri.Active > before+2100*time.Millisecond {
		t.Fatalf("gap not capped: %v", ri.Active)
	}
}
