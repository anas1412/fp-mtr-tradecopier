# FundingPips Trade Copier

Copies trades from a master account to a slave account on FundingPips MatchTrader platform.

## Setup

### 1. Install dependencies
```bash
cd fundingpips-copier
go mod tidy
```

### 2. Edit config
```bash
nano config.yaml
```
Fill in your master and slave account credentials. Both accounts must be on the same FundingPips email or different emails — both work as long as they are valid FundingPips MatchTrader accounts.

### 3. Build
```bash
go build -o copier .
```

### 4. Run
```bash
./copier
# or with a custom config path:
./copier /path/to/config.yaml
```

### 5. Run as systemd service (autostart on login)
```bash
# Create the directory
mkdir -p ~/.local/bin/fundingpips-copier

# Copy binary and files
cp copier ~/.local/bin/fundingpips-copier/
cp config.yaml ~/.local/bin/fundingpips-copier/

# Install the service
mkdir -p ~/.config/systemd/user
cp fundingpips-copier.service ~/.config/systemd/user/

# Enable and start
systemctl --user daemon-reload
systemctl --user enable fundingpips-copier
systemctl --user start fundingpips-copier

# Watch logs live
journalctl --user -u fundingpips-copier -f
```

## How it works

1. Logs into both accounts on startup
2. Seeds existing master positions (so they are NOT copied — only new ones are)
3. Polls master account every 500ms (configurable)
4. When a new position appears on master → opens same position on slave (with lot multiplier applied)
5. When a position closes on master → finds matching position on slave and closes it

## Config options

| Field | Description |
|---|---|
| `lot_multiplier` | Scale slave lot size. `1.0` = same, `0.5` = half, `2.0` = double |
| `poll_interval_ms` | How often to check master positions in milliseconds |
| `system_uuid` | FundingPips MatchTrader system UUID (hardcoded, no need to change) |
| `broker_id` | FundingPips broker ID (hardcoded as `"1"`, no need to change) |

## Notes

- The copier does NOT copy pre-existing positions when it starts — only new ones opened after the copier is running
- If the slave account has multiple open positions in the same symbol/direction, the close logic will close the first match
- Tokens from the login response are long-lived JWTs — no refresh needed
