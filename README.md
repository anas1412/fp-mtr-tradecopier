# FundingPips Trade Copier

Copies trades from a master account to a slave account on the FundingPips MatchTrader platform.

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
3. **Select slave** — the account to copy trades **to**
4. **Set multiplier** — scale the lot size (1.0 = same, 0.5 = half, 2.0 = double)
5. **Copying** — live copier running, showing positions and activity log

Credentials are saved to `copier-config.json` so you don't need to re-enter them next time. Press `l` during copying to log out and clear saved credentials.

### Controls

| Key | Action |
|---|---|
| `Tab` / `↑↓` | Navigate between fields |
| `Enter` / `Space` | Confirm / select |
| `Esc` / `b` | Go back |
| `q` | Quit |
| `l` | Logout (clear saved credentials) |

## How it works

1. Logs into both accounts on startup
2. Seeds existing master positions (so they are **not** copied — only new ones are)
3. Polls master account every 200ms
4. When a new position appears on master → opens the same position on slave (with lot multiplier applied)
5. When a position closes on master → finds matching position on slave and closes it
6. Both master and slave accounts can be on the same FundingPips email or different ones

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

- Pre-existing positions are **not** copied on startup — only new ones opened while the copier is running
- If the slave has multiple open positions on the same symbol/direction, the close logic matches the first one found
- Tokens from the login response are long-lived JWTs — no refresh needed
