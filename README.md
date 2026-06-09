# FundingPips Trade Copier

Copies trades (positions + pending limit/stop orders) from a master account to **one or more slave accounts** on the FundingPips MatchTrader platform.

## Quick Install

**Prerequisite:** [Go](https://go.dev/dl/) 1.22+

```bash
git clone git@github.com:anas1412/fp-mtr-tradecopier.git
cd fp-mtr-tradecopier
bash install.sh
```

This fetches dependencies and builds the `copier` binary.

## Usage

```bash
./copier
```

A terminal UI will guide you through:

1. **Login** — enter your FundingPips email and password
2. **Select master** — the account to copy trades **from**
3. **Select slave(s)** — one or more accounts to copy trades **to** (continue adding until you press "Done")
4. **Set multiplier per slave** — scale the lot size (1.0 = same, 0.5 = half, 2.0 = double)
5. **Copying** — live copier running, showing positions and activity log

Credentials are saved to `copier-config.json` so you don't need to re-enter or re-select accounts next time. If the saved master and slaves still exist on the server, the copier skips straight to copying.

### Controls (copying screen)

| Key | Action |
|---|---|
| `q` | Quit |
| `p` | Pause / resume copying |
| `l` | Logout (clear saved credentials) |
| `e` | Edit slaves (add/remove/change multipliers) |
| `s` | Settings (change poll interval) |

### Controls (other screens)

| Key | Action |
|---|---|
| `Tab` / `↑↓` | Navigate between fields |
| `Enter` / `Space` | Confirm / select |
| `Esc` / `b` | Go back |

## How it works

1. Logs into all accounts on startup
2. Seeds existing master positions and pending orders (they are **not** copied — only new ones are)
3. Polls master account for open positions and pending orders at a configurable interval (default: 500ms)
4. When a new position opens on master → opens the same position on all slaves (with per-slave lot multiplier)
5. When a position closes on master → finds matching position on each slave and closes it
6. When a pending (limit/stop) order appears on master → creates the same order on all slaves
7. When a pending order cancels on master → cancels the matching order on each slave
8. Both master and slave accounts can be on the same FundingPips email or different ones

## Settings

Press `s` during copying to open settings. Currently configurable:

- **Poll interval (ms)** — how often the copier checks for changes. Lower = faster copy, higher = less CPU. Min 50ms. Persisted across restarts.

## Autostart (systemd)

```bash
bash install.sh
mkdir -p ~/.local/bin/fundingpips-copier
cp copier ~/.local/bin/fundingpips-copier/
```

Create `~/.config/systemd/user/fundingpips-copier.service`:

```ini
[Unit]
Description=FundingPips Trade Copier
After=network.target

[Service]
ExecStart=%h/.local/bin/fundingpips-copier/copier
WorkingDirectory=%h/.local/bin/fundingpips-copier
Restart=on-failure

[Install]
WantedBy=default.target
```

Enable and start:

```bash
systemctl --user daemon-reload
systemctl --user enable fundingpips-copier
systemctl --user start fundingpips-copier
journalctl --user -u fundingpips-copier -f
```

## Notes

- Pre-existing positions and pending orders are **not** copied on startup — only new ones opened while the copier is running
- If a slave has multiple open positions on the same symbol/direction, the close logic matches the first one found
- Tokens from the login response are long-lived JWTs — no refresh needed
- The copier generates a stable `browserId` (UUID) on first run to present a consistent browser fingerprint to the server
