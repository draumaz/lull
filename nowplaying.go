// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"os/exec"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// The Silk streams don't publish what's playing, but every stream burns the
// artist and title into the bottom-left of the video. Every so often we grab a
// single low-res frame, crop that corner down to its white text and OCR it.

const (
	trackPollEvery = 25 * time.Second
	trackFormat    = "230" // 640x360 video-only: small, still sharp enough to read
)

// The overlay text sits right of the cover art, a little above the bottom
// edge. A steep contrast ramp turns the near-white text dark and washes out
// the busy background; a hard threshold breaks up the thin title font.
const overlayFilter = "crop=iw*0.55:ih*0.13:iw*0.15:ih*0.80," +
	"scale=iw*3:-1:flags=lanczos,format=gray,lut=y='clip((225-val)*3,0,255)'"

type track struct{ Artist, Title string }

func (t track) empty() bool { return t.Artist == "" }

type (
	trackPollMsg struct{ gen int }
	trackMsg     struct {
		gen   int
		track track
	}
)

// ocrAvailable reports whether ffmpeg, tesseract and its English data are
// installed.
func ocrAvailable() bool {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return false
	}
	out, err := exec.Command("tesseract", "--list-langs").CombinedOutput()
	return err == nil && slices.Contains(strings.Fields(string(out)), "eng")
}

func pollTrackAfter(gen int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return trackPollMsg{gen} })
}

// readTrack OCRs the now-playing overlay of a stream. It returns an empty
// track if anything goes wrong or the overlay isn't on screen.
func readTrack(gen int, videoURL string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// HLS URLs expire, so look it up fresh each time; it's cheap.
		u, err := exec.CommandContext(ctx, "yt-dlp", "-g", "-f", trackFormat, videoURL).Output()
		if err != nil {
			return trackMsg{gen: gen}
		}
		img, err := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error",
			"-i", strings.TrimSpace(string(u)), "-frames:v", "1",
			"-vf", overlayFilter, "-f", "image2pipe", "-c:v", "png", "-").Output()
		if err != nil {
			return trackMsg{gen: gen}
		}
		ocr := exec.CommandContext(ctx, "tesseract", "stdin", "stdout", "-l", "eng", "--psm", "6")
		ocr.Stdin = bytes.NewReader(img)
		text, err := ocr.Output()
		if err != nil {
			return trackMsg{gen: gen}
		}
		return trackMsg{gen: gen, track: parseOverlay(string(text))}
	}
}

// parseOverlay takes the first two plausible lines of OCR output as artist
// and title.
func parseOverlay(text string) track {
	var lines []string
	for l := range strings.Lines(text) {
		l = strings.Join(strings.Fields(l), " ")
		if plausible(l) {
			lines = append(lines, l)
		}
	}
	if len(lines) < 2 {
		return track{}
	}
	return track{Artist: titleCase(lines[0]), Title: titleCase(lines[1])}
}

// Words left lowercase mid-name: articles, short prepositions and collab
// markers, so "SENSE OF ADVENTURE" reads "Sense of Adventure".
var smallWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "of": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "by": true,
	"with": true, "x": true, "vs": true, "vs.": true, "feat.": true, "ft.": true,
}

// titleCase turns the overlay's all-caps text into title case. Letters after
// a hyphen, bracket or slash start a new word ("LO-FI (ORIGINAL MIX)" →
// "Lo-Fi (Original Mix)"); apostrophes don't ("DON'T" → "Don't").
func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if i > 0 && i < len(words)-1 && smallWords[w] {
			continue
		}
		r := []rune(w)
		start := true
		for j, c := range r {
			if start && unicode.IsLetter(c) {
				r[j] = unicode.ToUpper(c)
			}
			start = strings.ContainsRune("-([/\"", c)
		}
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// plausible weeds out OCR noise: real lines are mostly letters.
func plausible(s string) bool {
	letters := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return letters >= 2 && letters*10 >= len([]rune(s))*6
}
