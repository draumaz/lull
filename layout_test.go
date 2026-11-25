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
