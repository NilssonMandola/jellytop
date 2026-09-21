# jellytop

A terminal admin console for [Jellyfin](https://jellyfin.org). Live sessions, the
activity log, users, libraries and scheduled tasks — without leaving the shell.

Every other Jellyfin TUI is a *player*. This one is the dashboard.

```
jellytop  muse · 10.11.11 · http://127.0.0.1:8096                                    15:42:03
 1 Sessions   2 Activity   3 Users   4 Libraries   5 Tasks

  USER       NOW PLAYING                        DEVICE              STREAM
▶ ada        Severance S01E03 — In Perpetuity   Jellyfin Web · Mac  Direct Play  ████████░░░░  24:11/45:02
⏸ linus      Dune: Part Two (2024)              Infuse · Apple TV   Transcode    ███░░░░░░░░░  18:40/2:46:09
○ grace      idle                               Jellyfin · iPhone   —                          seen 3h

3 sessions · 2 streaming                                                          updated 1s ago
enter details  ·  s stop playback  ·  m message  ·  r refresh  ·  tab next  ·  ? help  ·  q quit
```

## What it does

| Tab | Shows | Actions |
|---|---|---|
| **Sessions** | who's connected, what's playing, direct play vs transcode, live progress | stop playback, send a message to a client, full session detail |
| **Activity** | the server activity log, paged and filterable | filter, page, per-entry detail |
| **Users** | accounts, admin/disabled state, last login and last seen | enable/disable, grant/revoke admin |
| **Libraries** | configured libraries, their paths and refresh state | trigger a scan |
| **Tasks** | scheduled tasks, last result, live progress | run now, cancel a running task |

Sessions refresh every 2s, tasks every 2s while one is running. Only the visible
tab polls, so an idle jellytop costs the server one request every few seconds.

Every action that changes server state asks first.

## Install

```sh
go install github.com/NilssonMandola/jellytop@latest
```

Or build it yourself:

```sh
git clone https://github.com/NilssonMandola/jellytop
cd jellytop
go build .
```

## Setup

jellytop authenticates with a Jellyfin API key. In Jellyfin:
**Dashboard → Advanced → API Keys → +**.

Then either export it:

```sh
export JELLYFIN_URL=http://127.0.0.1:8096
export JELLYFIN_TOKEN=<key>
jellytop
```

…or write `~/.config/jellytop/config`:

```ini
url   = http://127.0.0.1:8096
token = <key>
```

Flags beat the environment, which beats the config file:

```sh
jellytop -url https://jf.example.com -token <key>
```

The key needs administrator rights — the activity log, user policies and
scheduled tasks are all admin-only endpoints.

## Keys

| Key | Does |
|---|---|
| `1`…`5` | jump to a tab |
| `tab` / `shift+tab` | cycle tabs |
| `j` / `k`, arrows | move the cursor |
| `g` / `G` | top / bottom |
| `pgup` / `pgdn` | page |
| `enter` | detail view, or run the selected task |
| `r` | refresh now |
| `?` | help |
| `q` | quit |

Per tab: `s` stop playback and `m` message a client (Sessions) · `/` filter and
`n`/`p` page (Activity) · `d` enable/disable and `a` toggle admin (Users) ·
`S` scan libraries (Libraries) · `x` cancel a running task (Tasks).

## Notes and limits

- **Activity filtering is page-local.** `/` filters the 100 entries currently
  loaded, not the whole log — Jellyfin's activity endpoint has no search
  parameter.
- **Library scans are all-or-nothing.** Jellyfin exposes no per-library scan, so
  `S` triggers a scan of every library.
- **No live now-playing history.** jellytop shows the present and the activity
  log; for long-term playback statistics use
  [Streamystats](https://github.com/fredrikburmester/streamystats) or
  [Jellystat](https://github.com/CyferShepard/Jellystat).
- Tested against Jellyfin 10.11. The client models only the fields it renders,
  so a newer server adding fields is harmless.

## Prior art

[jellyctl](https://github.com/sj14/jellyctl) and
[JellyRoller](https://github.com/LSchallot/JellyRoller) cover similar ground as
scriptable CLIs — reach for those in scripts, and jellytop when you want to
watch.

## License

MIT — see [LICENSE](LICENSE).
