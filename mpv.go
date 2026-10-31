package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// mpv is a tiny client for a headless mpv instance driven over its JSON IPC
// socket. Events coming back from mpv are forwarded to the events channel.
type mpv struct {
	cmd    *exec.Cmd
	conn   net.Conn
	sock   string
	mu     sync.Mutex
	events chan mpvEvent
}

type mpvEvent struct {
	Event  string          `json:"event"`
	Name   string          `json:"name"`
	Data   json.RawMessage `json:"data"`
	Reason string          `json:"reason"`
	Error  string          `json:"file_error"`
}

func startMPV(volume int) (*mpv, error) {
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("lull-%d.sock", os.Getpid()))
	_ = os.Remove(sock)

	cmd := exec.Command("mpv",
		"--idle=yes",
		"--no-video",
		"--no-terminal",
		"--force-window=no",
		"--ytdl-format=bestaudio/best",
		"--cache=yes",
		fmt.Sprintf("--volume=%d", volume),
		"--input-ipc-server="+sock,
	)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("couldn't start mpv: %w", err)
	}

	var conn net.Conn
	var err error
	for range 100 {
		if conn, err = net.Dial("unix", sock); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("couldn't connect to mpv: %w", err)
	}

	m := &mpv{cmd: cmd, conn: conn, sock: sock, events: make(chan mpvEvent, 64)}
	go m.read()

	for i, prop := range []string{"pause", "volume", "mute", "media-title", "paused-for-cache"} {
		m.send("observe_property", i+1, prop)
	}
	return m, nil
}

func (m *mpv) read() {
	sc := bufio.NewScanner(m.conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var ev mpvEvent
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.Event != "" {
			m.events <- ev
		}
	}
	close(m.events)
}

func (m *mpv) send(args ...any) {
	b, _ := json.Marshal(map[string]any{"command": args})
	m.mu.Lock()
	defer m.mu.Unlock()
	_, _ = m.conn.Write(append(b, '\n'))
}

func (m *mpv) load(url string)     { m.send("loadfile", url, "replace") }
func (m *mpv) togglePause()        { m.send("cycle", "pause") }
func (m *mpv) toggleMute()         { m.send("cycle", "mute") }
func (m *mpv) addVolume(delta int) { m.send("add", "volume", delta) }

func (m *mpv) close() {
	m.send("quit")
	done := make(chan struct{})
	go func() { _ = m.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		_ = m.cmd.Process.Kill()
	}
	_ = m.conn.Close()
	_ = os.Remove(m.sock)
}
