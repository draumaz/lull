// SPDX-License-Identifier: GPL-3.0-or-later

// lull — a cozy little terminal radio for Monstercat Silk's 24/7 livestreams.
package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"image/color"
	"math/rand/v2"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	startVolume = 60

	// the card grows with the terminal, within reason
	minCardWidth = 46
	maxCardWidth = 120
	minVizRows   = 3
	maxVizRows   = 12
)

// ── keys ────────────────────────────────────────────────────────────────────

type keyMap struct {
	Switch, Pause, Up, Down, Mute, Quit key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Switch, k.Pause, k.Up, k.Down, k.Mute, k.Quit}
}
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

var keys = keyMap{
	Switch: key.NewBinding(key.WithKeys("tab", "left", "right", "h", "l", "1", "2"), key.WithHelp("tab", "switch")),
	Pause:  key.NewBinding(key.WithKeys("space", "p"), key.WithHelp("space", "pause")),
	Up:     key.NewBinding(key.WithKeys("+", "=", "up", "k", "pgup"), key.WithHelp("+/pgup", "louder")),
	Down:   key.NewBinding(key.WithKeys("-", "_", "down", "j", "pgdown"), key.WithHelp("-/pgdn", "softer")),
	Mute:   key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mute")),
	Quit:   key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "bye")),
}

// ── messages ────────────────────────────────────────────────────────────────

type (
	resolvedMsg []channel
	mpvMsg      mpvEvent
	mpvGoneMsg  struct{}
	frameMsg    time.Time
)

func waitForMPV(p *mpv) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-p.events
		if !ok {
			return mpvGoneMsg{}
		}
		return mpvMsg(ev)
	}
}

func frame() tea.Cmd {
	return tea.Tick(110*time.Millisecond, func(t time.Time) tea.Msg { return frameMsg(t) })
}

// ── model ───────────────────────────────────────────────────────────────────

type state int

const (
	tuning state = iota
	buffering
	playing
	paused
	failed
)

type model struct {
	player   *mpv
	chans    []channel
	active   int
	resolved bool

	state      state
	mediaTitle string
	volume     int
	muted      bool
	since      time.Time
	errMsg     string

	ocr      bool  // can we read the now-playing overlay?
	trackGen int   // bumped on every tune-in so stale OCR results are dropped
	track    track // what's playing, as read off the video
	pending  track // a new reading waiting for a second opinion

	bars    []float64
	targets []float64

	spin          spinner.Model
	help          help.Model
	width, height int
}

func newModel(p *mpv, active, volume int) model {
	h := help.New()
	h.ShortSeparator = "  ·  "
	h.Styles.ShortKey = fg(lipgloss.Color("#C9C3E6")).Bold(true)
	h.Styles.ShortDesc = fg(lipgloss.Color("#7D7896"))
	h.Styles.ShortSeparator = fg(lipgloss.Color("#4A4660"))

	m := model{
		player: p,
		chans:  defaultChannels(),
		active: active,
		volume: volume,
		spin:   spinner.New(spinner.WithSpinner(spinner.Points)),
		help:   h,
		ocr:    ocrAvailable(),
	}
	m.resizeBars()
	return m
}

// layout is how much of the view fits in the terminal. As the window shrinks
// the decoration goes first (padding, blank lines, the visualizer, borders)
// so the parts that matter stay on screen.
type layout struct {
	cardW, inner int
	border       bool
	padX, padY   int
	gaps         bool // blank lines between sections
	tabs         bool
	bigTabs      bool // bordered tabs with taglines, rather than one line
	nowLines     int
	viz          int // visualizer rows; 0 hides it
	help         bool
	helpGap      bool // blank line between the card and the help
}

func (l layout) height() int {
	sections := []int{1, l.nowLines, 1} // header, now playing, status
	if l.tabs {
		sections = append(sections, cond(l.bigTabs, 4, 1))
	}
	if l.viz > 0 {
		sections = append(sections, l.viz)
	}
	h := 0
	for _, s := range sections {
		h += s
	}
	if l.gaps {
		h += len(sections) - 1
	}
	h += 2*l.padY + cond(l.border, 2, 0)
	h += cond(l.help, 1, 0) + cond(l.help && l.helpGap, 1, 0)
	return h
}

func (m model) layout() layout {
	w, h := m.width, m.height
	if w == 0 || h == 0 {
		w, h = minCardWidth, 100
	}

	l := layout{
		border: true, padX: 2, padY: 1, gaps: true,
		tabs: true, bigTabs: true, nowLines: 2, help: true, helpGap: true,
	}
	// the card is roughly 90% of the terminal, but never wider than it
	l.cardW = min(max(w*9/10, minCardWidth), maxCardWidth, w)
	setInner := func() { l.inner = max(1, l.cardW-2*l.padX-cond(l.border, 2, 0)) }
	setInner()
	if l.inner < 30 {
		l.padX = 1
		setInner()
	}
	if l.inner < 20 {
		l.border, l.padX = false, 0
		setInner()
	}

	// bordered tabs need room for their label plus the border
	need := 0
	for _, c := range m.chans {
		need = max(need, lipgloss.Width(withIcon(c.Icon, c.Name)), lipgloss.Width(c.Tagline))
	}
	if l.inner/len(m.chans) < need+2 {
		l.bigTabs = false
	}

	minViz := minVizRows
	fits := func() bool {
		l.viz = 0
		spare := h - l.height() - cond(l.gaps, 1, 0)
		l.viz = min(max(spare, 0), maxVizRows)
		return l.viz >= minViz && l.height() <= h
	}
	// least important first
	for _, shrink := range []func(){
		func() { l.padY = 0 },
		func() { l.helpGap = false },
		func() { l.bigTabs = false },
		func() { l.gaps = false },
		func() { minViz = 1 },
		func() { minViz = 0 },
		func() { l.help = false },
		func() { l.nowLines = 1 },
		func() { l.border, l.padX = false, 0; setInner() },
		func() { l.tabs = false },
	} {
		if fits() {
			break
		}
		shrink()
	}
	if !fits() || minViz < minVizRows {
		return l
	}
	// a tall visualizer can spare rows for breathing room it squeezed out
	for _, luxury := range []func(*layout){
		func(l *layout) { l.gaps = true },
		func(l *layout) { l.helpGap = true },
		func(l *layout) { l.padY = cond(l.gaps, 1, 0) }, // padding without gaps looks lopsided
	} {
		prev := l
		luxury(&l)
		if !fits() {
			l = prev
		}
	}
	return l
}

func cond[T any](ok bool, a, b T) T {
	if ok {
		return a
	}
	return b
}

// clip cuts a (possibly styled) string down to w cells.
func clip(s string, w int) string { return ansi.Truncate(s, max(w, 0), "…") }

// resizeBars matches the bar count to the card width, keeping current levels
// so the visualizer doesn't jump.
func (m *model) resizeBars() {
	n := m.layout().inner
	if n == len(m.bars) {
		return
	}
	bars, targets := make([]float64, n), make([]float64, n)
	copy(bars, m.bars)
	copy(targets, m.targets)
	m.bars, m.targets = bars, targets
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return resolvedMsg(resolveStreams(m.chans)) },
		waitForMPV(m.player),
		m.spin.Tick,
		frame(),
	)
}

func (m model) ch() channel { return m.chans[m.active] }

func (m *model) tuneIn() tea.Cmd {
	m.state, m.mediaTitle, m.errMsg = tuning, "", ""
	m.player.load(m.ch().URL())

	m.trackGen++
	m.track, m.pending = track{}, track{}
	if !m.ocr {
		return nil
	}
	return readTrack(m.trackGen, m.ch().URL())
}

// setTrack only switches to a new reading once two polls agree, which rides
// out the overlay fading between songs and the odd OCR misread.
func (m *model) setTrack(t track) {
	switch {
	case t.empty() || t == m.track:
		m.pending = track{}
	case m.track.empty() || t == m.pending:
		m.track, m.pending = t, track{}
	default:
		m.pending = t
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeBars()
		m.help.SetWidth(msg.Width)

	case resolvedMsg:
		m.chans = msg
		m.resolved = true
		return m, m.tuneIn()

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Switch):
			if !m.resolved {
				break
			}
			next := (m.active + 1) % len(m.chans)
			switch msg.String() {
			case "1":
				next = 0
			case "2":
				next = 1
			}
			if next != m.active || m.state == failed {
				m.active = next
				return m, m.tuneIn()
			}
		case key.Matches(msg, keys.Pause):
			m.player.togglePause()
		case key.Matches(msg, keys.Up):
			m.player.addVolume(5)
		case key.Matches(msg, keys.Down):
			m.player.addVolume(-5)
		case key.Matches(msg, keys.Mute):
			m.player.toggleMute()
		}

	case mpvMsg:
		m.handleMPV(mpvEvent(msg))
		return m, waitForMPV(m.player)

	case trackPollMsg:
		if msg.gen == m.trackGen {
			return m, readTrack(m.trackGen, m.ch().URL())
		}

	case trackMsg:
		if msg.gen != m.trackGen {
			break
		}
		m.setTrack(msg.track)
		return m, pollTrackAfter(m.trackGen, trackPollEvery)

	case mpvGoneMsg:
		m.state = failed
		m.errMsg = "mpv went away :("

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case frameMsg:
		m.animate()
		return m, frame()
	}
	return m, nil
}

func (m *model) handleMPV(ev mpvEvent) {
	switch ev.Event {
	case "start-file":
		if m.state != paused {
			m.state = tuning
		}
	case "playback-restart":
		if m.state != paused {
			m.state = playing
		}
		if m.since.IsZero() {
			m.since = time.Now()
		}
	case "end-file":
		if ev.Reason == "error" {
			m.state = failed
			m.errMsg = "couldn't reach the stream"
			if ev.Error != "" {
				m.errMsg += " (" + ev.Error + ")"
			}
		}
	case "property-change":
		switch ev.Name {
		case "pause":
			var p bool
			_ = json.Unmarshal(ev.Data, &p)
			if p {
				m.state = paused
			} else if m.state == paused {
				m.state = playing
			}
		case "paused-for-cache":
			var b bool
			_ = json.Unmarshal(ev.Data, &b)
			if b && m.state == playing {
				m.state = buffering
			} else if !b && m.state == buffering {
				m.state = playing
			}
		case "volume":
			var v float64
			if json.Unmarshal(ev.Data, &v) == nil {
				if vol := int(v + 0.5); vol != m.volume {
					m.volume = vol
					saveVolume(vol)
				}
			}
		case "mute":
			_ = json.Unmarshal(ev.Data, &m.muted)
		case "media-title":
			var t string
			if json.Unmarshal(ev.Data, &t) == nil && !strings.HasPrefix(t, "watch?v=") {
				m.mediaTitle = tidyTitle(t)
			}
		}
	}
}

// animate drifts each visualizer bar towards a randomly wandering target, so
// it breathes like music without needing to actually analyse the audio.
func (m *model) animate() {
	live := m.state == playing && !m.muted
	for i := range m.bars {
		if live {
			if rand.Float64() < 0.35 {
				// gentle bass-heavy curve with some sparkle up top
				shape := 0.55 + 0.45*(1-float64(i)/float64(len(m.bars)))
				m.targets[i] = shape * (0.25 + 0.75*rand.Float64())
			}
		} else {
			m.targets[i] = 0.04
		}
		m.bars[i] += (m.targets[i] - m.bars[i]) * 0.35
	}
}

// ── view ────────────────────────────────────────────────────────────────────

var (
	ink    = lipgloss.Color("#EDE9FF")
	muted  = lipgloss.Color("#8A84A6")
	faint  = lipgloss.Color("#4A4660")
	danger = lipgloss.Color("#FF8FA3")
)

func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// withIcon puts two spaces after an icon. Glyphs like ≋ ❀ ♡ ♪ take one cell
// but many fonts draw them wider, so a single space lets them bleed into the
// text that follows.
func withIcon(icon, text string) string { return icon + "  " + text }

func (m model) View() tea.View {
	ch := m.ch()
	accent := ch.Colors[0]
	l := m.layout()
	inner := l.inner

	sections := []string{m.header(accent, inner)}
	if l.tabs {
		sections = append(sections, m.tabs(l))
	}
	sections = append(sections, m.nowPlaying(l))
	if l.viz > 0 {
		sections = append(sections, m.visualizer(l.viz))
	}
	sections = append(sections, m.statusLine(inner))
	if l.gaps {
		spaced := make([]string, 0, 2*len(sections))
		for i, s := range sections {
			if i > 0 {
				spaced = append(spaced, "")
			}
			spaced = append(spaced, s)
		}
		sections = spaced
	}

	card := lipgloss.NewStyle().Padding(l.padY, l.padX).Width(l.cardW)
	if l.border {
		card = card.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Darken(accent, 0.25))
	}
	ui := card.Render(lipgloss.JoinVertical(lipgloss.Left, sections...))

	if l.help {
		h, full := m.help, m.help
		full.SetWidth(0)
		if lipgloss.Width(full.View(keys)) > m.width {
			h.ShortSeparator = " · " // tighten up before dropping keys
		}
		parts := []string{ui}
		if l.helpGap {
			parts = append(parts, "")
		}
		// help gives up on the ellipsis, and the width, when "…" won't fit
		ui = lipgloss.JoinVertical(lipgloss.Center, append(parts, clip(h.View(keys), m.width))...)
	}

	if m.width > 0 && m.height > 0 {
		ui = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, ui)
		// last resort for absurdly small windows: crop rather than scroll
		ui = lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(ui)
	}

	v := tea.NewView(ui)
	v.AltScreen = true
	v.WindowTitle = "lull · " + strings.ToLower(ch.Name)
	return v
}

// header is the station name with the live badge pushed to the right; the
// name shortens before the badge gets crowded out.
func (m model) header(accent color.Color, width int) string {
	icon := fg(accent).Render(m.ch().Icon)
	badge := m.badge(accent)
	room := width - lipgloss.Width(badge) - 1

	title := icon
	for _, name := range []string{"monstercat silk", "silk"} {
		if t := withIcon(icon, fg(accent).Bold(true).Render(name)); lipgloss.Width(t) <= room {
			title = t
			break
		}
	}
	gap := max(1, width-lipgloss.Width(title)-lipgloss.Width(badge))
	return clip(title+strings.Repeat(" ", gap)+badge, width)
}

// nowPlaying always takes the same number of lines, so the layout doesn't
// jump when a title wraps or the track changes.
func (m model) nowPlaying(l layout) string {
	box := lipgloss.NewStyle().Width(l.inner).Height(l.nowLines).MaxHeight(l.nowLines)
	if m.track.empty() {
		return box.Foreground(ink).Render(cmp.Or(m.mediaTitle, m.ch().Title))
	}
	artist := fg(ink).Bold(true).Render(m.track.Artist)
	title := fg(muted).Render(m.track.Title)
	if l.nowLines == 1 {
		return box.Render(clip(artist+fg(faint).Render(" · ")+title, l.inner))
	}
	return box.Render(clip(artist, l.inner) + "\n" + clip(title, l.inner))
}

func (m model) badge(accent color.Color) string {
	switch m.state {
	case playing:
		return withIcon(fg(danger).Render("●"), fg(ink).Bold(true).Render("LIVE"))
	case paused:
		return fg(muted).Render(withIcon("❚❚", "paused"))
	case failed:
		return fg(danger).Render(withIcon("✕", "offline"))
	default:
		return fg(accent).Render(strings.TrimSpace(m.spin.View()))
	}
}

func (m model) tabs(l layout) string {
	tabWidth := l.inner / len(m.chans)
	var out []string
	for i, c := range m.chans {
		label := withIcon(c.Icon, c.Name)
		style := lipgloss.NewStyle().Width(tabWidth).Align(lipgloss.Center)
		if i == m.active {
			style = style.Foreground(c.Colors[0]).Bold(true)
		} else {
			style = style.Foreground(muted)
		}
		if !l.bigTabs {
			if i != m.active {
				style = style.Faint(true)
			}
			out = append(out, style.Render(clip(label, tabWidth)))
			continue
		}
		style = style.Border(lipgloss.RoundedBorder())
		if i == m.active {
			style = style.BorderForeground(c.Colors[0])
		} else {
			style = style.BorderForeground(faint)
		}
		tag := fg(lipgloss.Darken(style.GetForeground(), 0.15)).Faint(i != m.active).Italic(true).Render(c.Tagline)
		out = append(out, style.Render(lipgloss.JoinVertical(lipgloss.Center, label, tag)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, out...)
}

func (m model) visualizer(rows int) string {
	levels := []rune(" ▁▂▃▄▅▆▇█")
	colors := lipgloss.Blend1D(len(m.bars), m.ch().Colors...)
	if m.state != playing || m.muted {
		for i := range colors {
			colors[i] = faint
		}
	}

	lines := make([]string, rows)
	for r := range rows {
		var b strings.Builder
		for i, h := range m.bars {
			// how many eighths of this row are filled
			fill := int(h*float64(rows)*8+0.5) - (rows-1-r)*8
			fill = min(max(fill, 0), 8)
			if r == rows-1 && fill == 0 {
				fill = 1 // keep a soft baseline
			}
			b.WriteString(fg(colors[i]).Render(string(levels[fill])))
		}
		lines[r] = b.String()
	}
	return strings.Join(lines, "\n")
}

func (m model) statusLine(width int) string {
	dim := fg(muted)

	if m.state == failed {
		return fg(danger).Render(clip(m.errMsg+" · tab to retry", width))
	}

	var right string
	switch {
	case !m.resolved:
		right = "finding streams…"
	case m.state == tuning:
		right = "tuning in…"
	case m.state == buffering:
		right = "buffering…"
	case !m.since.IsZero():
		right = withIcon("♡", clock(time.Since(m.since)))
	}

	icon := "♪"
	volText := fmt.Sprintf("%d%%", m.volume)
	if m.muted {
		icon, volText = "∅", "muted"
	}

	// the volume meter gives up room first, then the right-hand status
	volFixed := lipgloss.Width(withIcon(icon, " "+volText))
	free := width - volFixed
	if right != "" {
		free -= lipgloss.Width(right) + 1
	}
	if free < 6 {
		right, free = "", width-volFixed
	}
	slots := min(max(width/4, 10), 24, max(free, 0))
	filled := min(slots, (m.volume*slots+50)/100)
	meter := fg(m.ch().Colors[1]).Render(strings.Repeat("━", filled)) +
		fg(faint).Render(strings.Repeat("━", slots-filled))
	vol := withIcon(dim.Render(icon), meter+dim.Render(" "+volText))

	if right == "" {
		return clip(vol, width)
	}
	right = dim.Render(right)
	gap := max(1, width-lipgloss.Width(vol)-lipgloss.Width(right))
	return clip(vol+strings.Repeat(" ", gap)+right, width)
}

// yt-dlp suffixes live titles with the current date and time; drop that and
// any doubled-up spaces.
var liveStamp = regexp.MustCompile(`\s+\d{4}-\d{2}-\d{2} \d{2}:\d{2}$`)

func tidyTitle(t string) string {
	return strings.Join(strings.Fields(liveStamp.ReplaceAllString(t, "")), " ")
}

func clock(d time.Duration) string {
	d = d.Round(time.Second)
	h, mnt, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, mnt, s)
	}
	return fmt.Sprintf("%02d:%02d", mnt, s)
}

// ── main ────────────────────────────────────────────────────────────────────

func main() {
	for _, bin := range []string{"mpv", "yt-dlp"} {
		if _, err := exec.LookPath(bin); err != nil {
			fmt.Fprintf(os.Stderr, "lull needs %s installed and on your PATH\n", bin)
			os.Exit(1)
		}
	}

	active := 0
	if len(os.Args) > 1 {
		switch strings.ToLower(os.Args[1]) {
		case "melodic", "m", "2":
			active = 1
		case "progressive", "p", "1":
			active = 0
		default:
			fmt.Fprintln(os.Stderr, "usage: lull [progressive|melodic]")
			os.Exit(2)
		}
	}

	volume := loadVolume()
	player, err := startMPV(volume)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer player.close()

	if _, err := tea.NewProgram(newModel(player, active, volume)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
