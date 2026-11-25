// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var baseModel = newModel(nil, 0, 60) // probes for tesseract, so only once

func testModel(w, h int) model {
	m := baseModel
	m.resolved, m.state, m.since = true, playing, time.Now()
	m.track = track{Artist: "Some Artist", Title: "A Fairly Long Track Title (Extended Mix)"}
	m.width, m.height = w, h
	m.resizeBars()
	m.help.SetWidth(w)
	return m
}

// Whatever the window size, the view must fit inside it, and the essentials
// must survive for as long as there's any reasonable room for them.
func TestViewFitsWindow(t *testing.T) {
	var widths []int
	for w := 10; w <= 90; w++ {
		widths = append(widths, w)
	}
	widths = append(widths, 120, 160, 220)
	for _, w := range widths {
		for h := 1; h <= 40; h += 1 + h/12 {
			m := testModel(w, h)
			out := m.View().Content
			if got := lipgloss.Height(out); got > h {
				t.Fatalf("%dx%d: view is %d rows tall", w, h, got)
			}
			if got := lipgloss.Width(out); got > w {
				t.Fatalf("%dx%d: view is %d cols wide", w, h, got)
			}
			if l := m.layout(); len(m.bars) != l.inner {
				t.Fatalf("%dx%d: %d bars for %d cols", w, h, len(m.bars), l.inner)
			}

			plain := ansi.Strip(out)
			want := []string{"LIVE"}
			if h >= 3 && w >= 16 {
				want = append(want, "Some Artist", "60%")
			}
			if h >= 8 && w >= 40 {
				want = append(want, "Progressive", "Melodic", "switch")
			}
			for _, s := range want {
				if !strings.Contains(plain, s) {
					t.Fatalf("%dx%d: %q missing from\n%s", w, h, s, plain)
				}
			}
		}
	}
}

// Clicking a tab's label must pick that tab, wherever the layout put it.
func TestClickTabs(t *testing.T) {
	for w := 20; w <= 220; w += 7 {
		for h := 4; h <= 40; h += 3 {
			m := testModel(w, h)
			if !m.layout().tabs {
				continue
			}
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			for i, c := range m.chans {
				name := c.Name[:4] // labels get cut short in narrow windows
				found := false
				for y, line := range lines {
					col := strings.Index(line, name)
					if col < 0 || strings.Contains(line, c.Title) {
						continue
					}
					found = true
					x := ansi.StringWidth(line[:col])
					for dx := range len(name) {
						if r, _ := m.regionAt(x+dx, y); r.what != onTab || r.i != i {
							t.Fatalf("%dx%d: click at %d,%d on %q hit %+v", w, h, x+dx, y, name, r)
						}
					}
				}
				if !found {
					t.Fatalf("%dx%d: %q not on screen", w, h, name)
				}
			}
			if r, _ := m.regionAt(0, 0); r.what != nowhere {
				t.Fatalf("%dx%d: top-left corner hit %+v", w, h, r)
			}
		}
	}
}

// The meter and help items must be clickable exactly where they're drawn.
func TestClickMeterAndHelp(t *testing.T) {
	for w := 16; w <= 220; w += 5 {
		for h := 3; h <= 40; h += 3 {
			m := testModel(w, h)
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			at := func(x, y int) string {
				if y < 0 || y >= len(lines) {
					return ""
				}
				return ansi.Cut(lines[y], x, x+1)
			}

			meter, ok := m.meter()
			if !ok {
				t.Fatalf("%dx%d: no volume meter", w, h)
			}
			for dx := range meter.w {
				if c := at(meter.x+dx, meter.y); c != "━" {
					t.Fatalf("%dx%d: meter cell %d is %q", w, h, dx, c)
				}
			}
			if c := at(meter.x+meter.w, meter.y); c == "━" {
				t.Fatalf("%dx%d: meter runs past its region", w, h)
			}

			for _, r := range m.regions() {
				if r.what != onHelp {
					continue
				}
				b := keys.ShortHelp()[r.i].Help()
				want := b.Key + " " + b.Desc
				if got := ansi.Cut(lines[r.y], r.x, r.x+r.w); got != want {
					t.Fatalf("%dx%d: help region %d covers %q, not %q", w, h, r.i, got, want)
				}
			}
		}
	}
}

func TestMeterVolume(t *testing.T) {
	for _, c := range []struct{ dx, w, want int }{
		{-1, 10, 0}, {0, 10, 10}, {4, 10, 50}, {9, 10, 100}, {20, 10, 100}, {0, 0, 0},
	} {
		if got := meterVolume(c.dx, c.w); got != c.want {
			t.Errorf("meterVolume(%d, %d) = %d, want %d", c.dx, c.w, got, c.want)
		}
	}
}
