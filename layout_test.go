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
func TestTabAt(t *testing.T) {
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
						if got := m.tabAt(x+dx, y); got != i {
							t.Fatalf("%dx%d: click at %d,%d on %q hit tab %d", w, h, x+dx, y, name, got)
						}
					}
				}
				if !found {
					t.Fatalf("%dx%d: %q not on screen", w, h, name)
				}
			}
			if got := m.tabAt(0, 0); got != -1 {
				t.Fatalf("%dx%d: top-left corner hit tab %d", w, h, got)
			}
		}
	}
}
