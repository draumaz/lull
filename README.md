# lull

A cozy little terminal radio for [Monstercat Silk](https://www.youtube.com/@MonstercatSilk)'s 24/7 livestreams.

lull plays the **Progressive House** and **Deep & Melodic House** streams as audio only, with a gently animated visualizer and the current artist and song title, which it reads straight off the stream's video.

```
╭─────────────────────────────────────────────────────────────╮
│                                                             │
│  ≋  monstercat silk                                ●  LIVE  │
│                                                             │
│  ╭──────────────────────────╮╭──────────────────────────╮   │
│  │      ≋  Progressive      ││        ❀  Melodic        │   │
│  │      sunny & bright      ││      deep & dreamy       │   │
│  ╰──────────────────────────╯╰──────────────────────────╯   │
│                                                             │
│  Shingo Nakamura & Hausman                                  │
│  Sense of Adventure                                         │
│                                                             │
│  ▃▅▆▇▆▅▇▆▄▅▆▅▄▃▅▄▃▄▅▃▂▃▄▃▂▃▂▂▃▂▁▂▃▂▁▂▁▂▁▁▂▁▁▂▁▁▁▂▁▁▁▁▁▁▁▁  │
│                                                             │
│  ♪  ━━━━━━━━━━━━━━ 60%                            ♡  01:30  │
│                                                             │
╰─────────────────────────────────────────────────────────────╯
   tab switch  ·  space pause  ·  +/pgup louder  ·  -/pgdn softer
```

## Dependencies

lull runs on **Linux** and **macOS**. Windows isn't supported: lull talks to mpv over a Unix socket.

### Required

| Dependency | Why |
|---|---|
| [Go](https://go.dev/dl/) 1.27.3 or newer | to build lull |
| [mpv](https://mpv.io/) | plays the audio |
| [yt-dlp](https://github.com/yt-dlp/yt-dlp) | finds the live streams and hands them to mpv |

### Optional: now-playing track names

The streams don't publish what's playing, so lull grabs a small video frame every 25 seconds and reads the artist and title from it with OCR. Without these, lull still plays and shows the stream's title instead.

| Dependency | Why |
|---|---|
| [ffmpeg](https://ffmpeg.org/) | grabs and crops a video frame |
| [Tesseract](https://github.com/tesseract-ocr/tesseract) | reads the text |
| Tesseract English language data | Tesseract can't read anything without a language pack; some distros ship it separately |

### A terminal that can show it

- A font with good Unicode coverage, for the box drawing, block and symbol characters.
- True-color support looks best; lull falls back gracefully on terminals with fewer colors.
- At least about 50 columns by 24 rows. lull grows to fill bigger windows.

### Install commands

**Arch Linux**
```sh
sudo pacman -S go mpv yt-dlp ffmpeg tesseract tesseract-data-eng
```

**macOS** ([Homebrew](https://brew.sh/); Homebrew's `tesseract` includes English)
```sh
brew install go mpv yt-dlp ffmpeg tesseract
```

**Debian / Ubuntu**
```sh
sudo apt install mpv ffmpeg tesseract-ocr tesseract-ocr-eng
```
The `golang` and `yt-dlp` packages in Debian and Ubuntu are usually too old. Install Go from [go.dev/dl](https://go.dev/dl/) and yt-dlp from its [releases page](https://github.com/yt-dlp/yt-dlp#installation) or with `pipx install yt-dlp`.

**Fedora** (ffmpeg needs [RPM Fusion](https://rpmfusion.org/Configuration))
```sh
sudo dnf install golang mpv yt-dlp ffmpeg tesseract tesseract-langpack-eng
```

YouTube changes often, and an out-of-date yt-dlp is the most common reason streams stop loading. Keep it updated.

## Install

With Go installed, this puts `lull` in `~/go/bin` (make sure that's on your `PATH`):

```sh
go install github.com/draumaz/lull@latest
```

Or build from source:

```sh
git clone https://github.com/draumaz/lull.git
cd lull
go build
./lull
```

## Usage

```sh
lull               # start on Progressive House
lull melodic       # start on Deep & Melodic House (also: m, 2)
lull progressive   # also: p, 1
```

| Keys | Action |
|---|---|
| `tab` `←` `→` `h` `l` | switch stream |
| `1` `2` | jump to Progressive / Melodic |
| `space` `p` | pause |
| `+` `=` `↑` `k` `pgup` | louder |
| `-` `_` `↓` `j` `pgdn` | softer |
| `m` | mute |
| `q` `esc` `ctrl+c` | quit |

lull remembers your volume between runs, in `~/.config/lull/volume` on Linux and `~/Library/Application Support/lull/volume` on macOS.

## How it works

- **Streams:** at startup lull asks yt-dlp which streams are live on the Monstercat Silk channel, so it keeps working if the 24/7 streams restart under new video IDs.
- **Playback:** a headless mpv plays audio only and reports pause, volume and buffering back to lull.
- **Track names:** every 25 seconds lull fetches one 360p frame, crops the artist/title overlay in the bottom-left corner and runs it through Tesseract. A new song only replaces the current one when two readings agree, which rides out the overlay fading between songs. The overlay is all caps, so lull converts names to title case; stylized names like "BT" will come out as "Bt".
- **Visualizer:** it's decorative. The bars drift toward random, bass-heavy targets while music is playing rather than analysing the audio.

## License

Copyright (C) 2026 Emma Anderson

lull is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version. See [LICENSE](LICENSE) for the full text.

## Disclaimer

lull is an unofficial fan project and isn't affiliated with or endorsed by Monstercat. All music belongs to its artists and Monstercat. Please support them on [Monstercat](https://www.monstercat.com/) and the [Monstercat Silk YouTube channel](https://www.youtube.com/@MonstercatSilk).
