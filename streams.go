package main

import (
	"context"
	"image/color"
	"os/exec"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

const silkStreams = "https://www.youtube.com/@MonstercatSilk/streams"

type channel struct {
	Name    string
	Tagline string
	Icon    string
	VideoID string // fallback; refreshed from the channel at startup
	Title   string
	Colors  []color.Color
	match   func(title string) bool
}

func (c channel) URL() string { return "https://www.youtube.com/watch?v=" + c.VideoID }

func defaultChannels() []channel {
	return []channel{
		{
			Name:    "Progressive",
			Tagline: "sunny & bright",
			Icon:    "≋",
			VideoID: "d8Oc90QevaI",
			Title:   "Progressive House 24/7",
			Colors:  []color.Color{lipgloss.Color("#7BE0D6"), lipgloss.Color("#8FB8FF"), lipgloss.Color("#C3A6FF")},
			match: func(t string) bool {
				return strings.Contains(strings.ToLower(t), "progressive")
			},
		},
		{
			Name:    "Melodic",
			Tagline: "deep & dreamy",
			Icon:    "❀",
			VideoID: "WsDyRAPFBC8",
			Title:   "Deep & Melodic House 24/7",
			Colors:  []color.Color{lipgloss.Color("#FFB3D1"), lipgloss.Color("#E7A6FF"), lipgloss.Color("#A9A6FF")},
			match: func(t string) bool {
				t = strings.ToLower(t)
				return strings.Contains(t, "melodic") && !strings.Contains(t, "progressive")
			},
		},
	}
}

// resolveStreams asks yt-dlp which streams are currently live on Monstercat
// Silk, so we keep working if the 24/7 streams get restarted under new IDs.
func resolveStreams(chans []channel) []channel {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "yt-dlp", "--flat-playlist", "--no-warnings",
		"--print", "%(live_status)s\t%(id)s\t%(title)s", silkStreams).Output()
	if err != nil && len(out) == 0 {
		return chans
	}

	resolved := append([]channel(nil), chans...)
	found := make([]bool, len(resolved))
	for line := range strings.Lines(string(out)) {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 3)
		if len(parts) != 3 || parts[0] != "is_live" {
			continue
		}
		for i := range resolved {
			if !found[i] && resolved[i].match(parts[2]) {
				resolved[i].VideoID, resolved[i].Title = parts[1], parts[2]
				found[i] = true
				break
			}
		}
	}
	return resolved
}
