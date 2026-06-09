# FundingPips Copier — Architecture & Design Notes

> Project: `/home/blackbox/fundingpips-copier`
> Language: Go (single-file app: `main.go`)
> Platform: Linux TUI (terminal UI using Bubble Tea / `tea` library)
> Last updated: 2026-06-09

---

## What It Does

A **trade copier** for FundingPips MatchTrader platform. One master account's trades are automatically mirrored to N slave accounts with per-slave volume multipliers.

---

## Architecture

### Single Binary

Everything is in `main.go` — no packages, no modules split. This is intentional for simplicity (the user prefers simple).

### Key Types

| Type | Purpose |
|---|---|
| `model` | Main Bubble Tea model — holds all state (master/slave clients, positions, UI state) |
| `masterClient` | Wraps HTTP calls to MatchTrader API for the master account |
| `slaveClient` | Wraps HTTP calls for a single slave account |
| `SlaveConfig` | Persisted config: `{AccountID, Multiplier}` |
| `copierConfig` | Saved to `copier-config.json` |

### Config File: `copier-config.json`

```json
{
  "masterID": "account123",
  "slaves": [
    {"accountID": "slave456", "multiplier": 1.0},
    {"accountID": "slave789", "multiplier": 0.5}
  ],
  "masterToken": "...encrypted...",
  "slaveTokens": ["...encrypted...", "...encrypted..."]
}
```

**Migration note**: Old format used `slaveID` (single string) + `slaveToken`. The app auto-migrates to the `slaves[]` array format on startup.

### Volume Multiplier

Each slave has a multiplier applied to the master's volume:
```
slave_volume = master_volume × multiplier
```

Example: Master opens 0.10, slave has multiplier 2.5 → slave opens 0.25.

---

## Features Implemented

### ✅ Login Screen
- Server URL, master account ID + token, slave account(s) ID + token
- Saves to `copier-config.json` after successful connection
- **`l` key to load last config** (auto-fills from saved file)

### ✅ Multi-Slave (one master → N slaves)
- Any number of slaves
- Each has independent multiplier
- All slaves copy from the same master

### ✅ Position Polling + Copying
- Polls master's open positions every 3 seconds (`GET /mtr-api/{uuid}/open-positions`)
- Diffs against last known set
- Opens new positions on slaves (if not already present)
- Closes positions on slaves (when master closes)

### ✅ Pause / Resume (`p` key)
- Toggles copying on/off
- Shows `⏸ PAUSED` badge in UI when paused
- Does NOT clear state — resumes cleanly

### ✅ Edit Screen (`e` key during copying)
- Shows current slaves with multipliers
- Add new slave
- Remove a slave
- Change multiplier for existing slave
- Apply changes (only after hitting "Apply")
- Navigation: up/down arrows to select, enter to edit, `a` to add, `d` to delete, `Esc` to cancel

### ✅ Logout
- Clears saved credentials (`copier-config.json`)
- Returns to login screen

### ✅ UI Panels
- Master panel: balance, open positions count, pending orders count (planned)
- Per-slave panels: balance, multiplier, stats
- Status bar: pause state, copy/close counts, timestamp

---

## Pending Orders Feature (In Progress)

### Strategy

Same polling + diff approach as market positions:

1. Poll master's pending orders via `GET /mtr-api/{uuid}/active-orders`
2. Maintain `lastPendingOrders` map in model (keyed by order `id`)
3. On each poll:
   - **New orders** (in master, not in last) → `POST /mtr-api/{uuid}/pending-order/create` on each slave (volume × multiplier)
   - **Removed orders** (in last, not in master) → `POST /mtr-api/{uuid}/pending-order/cancel` on each slave
   - **Existing orders** update the stored set
4. Copy/close counts tracked separately from market positions

### API Endpoints Discovered

| Action | Endpoint | Method |
|---|---|---|
| List | `/mtr-api/{uuid}/active-orders` | `GET` |
| Create | `/mtr-api/{uuid}/pending-order/create` | `POST` |
| Cancel | `/mtr-api/{uuid}/pending-order/cancel` | `POST` |
| Edit | `/mtr-api/{uuid}/pending-orders/bulk-edit` | `POST` |

Full details in `matchtrader-api.md`.

### Pending Order Fields

| Field | Type | Notes |
|---|---|---|
| `id` | string | Unique order ID |
| `symbol` | string | e.g. "EURUSD" |
| `alias` | string | e.g. "EUR/USD" |
| `side` | `"BUY"` / `"SELL"` | |
| `type` | `"LIMIT"` / `"STOP"` | Pending order type |
| `volume` | float | Trade size |
| `activationPrice` | float | Trigger price |
| `creationTime` | number | Unix timestamp |
| `stopLoss` | float? | Optional |
| `takeProfit` | float? | Optional |

### Cancel Request Body

```
{id: "orderId", instrument: "EURUSD", orderSide: "BUY", type: "LIMIT"}
```

### Create Request Body

```
{instrument: "EURUSD", orderSide: "BUY", volume: 0.01, type: "LIMIT", price: 1.12, slPrice: null, tpPrice: null, isMobile: true}
```

---

## UI Structure

```
┌──────────────────────────────────────────────┐
│  FundingPips Trade Copier v1.2.0             │
│  Copier running...                ⏸ PAUSED  │
├──────────────────────────────────────────────┤
│  ┌────────────────┐  ┌────────────────────┐  │
│  │ MASTER #123    │  │ SLAVE #456 (×1.0)  │  │
│  │ Balance: $5k   │  │ Balance: $3k       │  │
│  │ Open: 3        │  │ Open: 3            │  │
│  │ Pending: 1     │  │ Pending: 1         │  │
│  │ Copied: 5      │  │ Copied: 5          │  │
│  │ Closed: 2      │  │ Closed: 2          │  │
│  └────────────────┘  └────────────────────┘  │
│  ┌────────────────┐  ┌────────────────────┐  │
│  │ SLAVE #789     │  │ [add more...]      │  │
│  │ (×0.5)         │  │                    │  │
│  │ ...            │  │                    │  │
│  └────────────────┘  └────────────────────┘  │
├──────────────────────────────────────────────┤
│  [p] pause  [e] edit  [l] logout  [q] quit  │
│  Positions copied: 5  Closed: 2              │
│  Pending copied: 1   Cancelled: 1            │
└──────────────────────────────────────────────┘
```

### Key Bindings

| Key | Screen | Action |
|---|---|---|
| `p` | Copying | Toggle pause/resume |
| `e` | Copying | Open edit screen |
| `l` | Copying | Logout (clear config) |
| `l` | Login | Load last config |
| `q` | Any | Quit |
| ↑↓ | Login/Edit | Navigate |
| `Enter` | Login | Connect / Edit |
| `a` | Edit | Add slave |
| `d` | Edit | Delete slave |
| `Esc` | Edit | Cancel changes |

---

## Data Flow

```
                ┌─────────────────┐
                │   Master API    │
                │  (poll every 3s)│
                └───────┬─────────┘
                        │
            ┌───────────┴────────────┐
            │    doPoll()             │
            │    - fetch positions    │
            │    - fetch pending      │
            │    - diff & sync        │
            └───────────┬────────────┘
                        │
            ┌───────────┴────────────┐
            │   For each slave...    │
            └───────────┬────────────┘
                        │
            ┌───────────┴────────────┐
            │   Slave API calls      │
            │   (open/close/copy)    │
            └────────────────────────┘
```

---

## Security Notes

- Tokens are stored in `copier-config.json` (no encryption — simple base64-ish encoding)
- No HTTPS enforcement in code (relies on server URL being https)
- No input validation on server URL
- All API communication uses the `Auth-Trading-Api` header
