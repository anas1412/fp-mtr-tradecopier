# MatchTrader API Reference

> Source: Reverse-engineered from `mtr-platform.fundingpips.com` webapp JS (v1.29.3-2290).
> Gathered: 2026-06-09.
> Base URL: `https://mtr-platform.fundingpips.com`

---

## Authentication

### Login (Credentials)

```
POST /mtr-core-edge/v2/oauth-login
Body:   {login, password, remember}
Return: {tradingToken, ...}
```

### Login (with TFA)

```
POST /mtr-core-edge/v2/oauth-login/login-with-tfa
Body:   {tfaCode, ...}
Return: {tradingToken, ...}
```

### Login (with existing token)

```
POST /mtr-core-edge/v2/login/with-token
Body:   {token}
Return: {tradingToken, ...}
```

### Token Refresh

```
POST /mtr-core-edge/refresh-token
```

### Auth Headers

Every `mtr-api` and `trading-edge` request requires:

| Header | Value |
|---|---|
| `Auth-Trading-Api` | From `selectedTradingAccountTradingApiToken()` |
| `Trading-Account-Token` | From `selectedTradingAccount().tradingAccountToken` |

---

## Open Positions

### List Open Positions

```
GET /mtr-api/{systemUUID}/open-positions
```

**Response:**
```json
[
  {
    "id": "string",
    "symbol": "EURUSD",
    "alias": "EUR/USD",
    "side": "BUY" | "SELL",
    "volume": 0.01,
    "openPrice": 1.12345,
    "currentPrice": 1.12400,
    "profit": 1.23,
    "commission": -0.5,
    "swap": -0.1,
    "stopLoss": null | 1.12000,
    "takeProfit": null | 1.13000,
    "openTime": 1234567890
  }
]
```

### Open a Position

```
POST /mtr-api/{systemUUID}/position/open
Body:
{
  "instrument": "EURUSD",
  "orderSide": "BUY" | "SELL",
  "volume": 0.01,
  "slPrice": null | 1.12000,
  "tpPrice": null | 1.13000,
  "isMobile": true
}
Return: {positionId: "string", status: "success"}
```

### Close Positions

```
POST /mtr-api/{systemUUID}/positions/close
Body:
{
  "ids": ["positionId1", "positionId2"]
}
Return: {status: "success"}
```

### Partial Close a Position

```
POST /mtr-api/{systemUUID}/position/partial-close
Body:  {positionId, volume, orderSide, instrument}
Return: {status: "success", ...}
```

### Edit Position (TP/SL)

```
POST /mtr-api/{systemUUID}/positions/edit
Body:  {id, slPrice, tpPrice, trailingDistance}
Return: {status: "success"}
```

### Bulk Edit Positions

```
POST /mtr-api/{systemUUID}/positions/bulk-edit
Body: [{id, slPrice, tpPrice}]
Return: {status: "success"}
```

---

## Pending Orders (Limit / Stop)

### List Pending Orders (CRITICAL — hard to discover)

```
GET /mtr-api/{systemUUID}/active-orders
```

**Response:**
```json
{
  "orders": [
    {
      "id": "string",
      "symbol": "EURUSD",
      "alias": "EUR/USD",
      "side": "BUY" | "SELL",
      "type": "LIMIT" | "STOP",
      "volume": 0.01,
      "activationPrice": 1.12000,
      "creationTime": 1234567890,
      "stopLoss": null | 1.11000,
      "takeProfit": null | 1.13000,
      "openPrice": null,
      "currentPrice": null
    }
  ]
}
```

**Note:** When using the Trading Edge backend instead of MTR API:
```
GET /trading-edge/{systemUUID}/pending-orders
```
Response: `{orders: [...]}` (same shape, but `creationTimeIso` instead of `creationTime`)

### Create Pending Order

```
POST /mtr-api/{systemUUID}/pending-order/create
Body:
{
  "instrument": "EURUSD",
  "orderSide": "BUY" | "SELL",
  "volume": 0.01,
  "type": "LIMIT" | "STOP",
  "price": 1.12000,           // the activation/trigger price
  "slPrice": null | 1.11000,
  "tpPrice": null | 1.13000,
  "isMobile": true
}
Return: {orderId: "string", status: "success"}
```

### Cancel Pending Order

```
POST /mtr-api/{systemUUID}/pending-order/cancel
Body:
{
  "id": "orderId",
  "instrument": "EURUSD",
  "orderSide": "BUY" | "SELL",
  "type": "LIMIT" | "STOP"
}
Return: {orderId: "string", status: "success"}
```

### Edit Pending Order (TP/SL/Price)

```
POST /mtr-api/{systemUUID}/pending-orders/bulk-edit
Body:
[
  {
    "id": "orderId",
    "volume": 0.01,
    "slPrice": null | 1.11000,
    "tpPrice": null | 1.13000,
    "price": 1.12000     // new activation price
  }
]
Return: {status: "success"}
```

---

## Account & Trading Info

### Instruments List

```
From WebSocket or instrument store — no direct REST endpoint observed.
Instrument entity map keyed by symbol.
Each instrument has:
  - symbol (e.g. "EURUSD")
  - alias (e.g. "EUR/USD")
  - volumePrecision (e.g. 2)
  - pricePrecision (e.g. 5)
  - pointValue (used for profit calculation)
  - sizeOfOnePoint
  - sessionOpen (bool)
  - closeReason (if closed, e.g. "HOLIDAY")
```

### Trading Account Info

```
GET /mtr-api/{systemUUID}/account-info
```
Or via the edge services — not fully traced.

### Price Alerts

```
Endpoint prefix: /mtr-api/{systemUUID}/price-alerts
```

---

## WebSocket (Live Price Feed)

Used for real-time price streaming. Typically connects via:

```
WebSocket to: mtr-core-edge/...
```

Messages include:
- `hs` (hot symbols)
- `hsp` (hot symbol prices)
- `pcd` (profit calculation data)

---

## Backend Variants

The webapp supports **two backends**:

| Backend | URL Prefix | When Used |
|---|---|---|
| **MTR API** (legacy) | `/mtr-api/{uuid}/...` | Default |
| **Trading Edge** | `/trading-edge/{uuid}/...` | When `partnerConfig.useTradingEdge` is true |

Both return similar data shapes. The copier currently uses only the MTR API.

---

## Error Responses

Errors from all endpoints typically follow this shape:

```json
{
  "error": {
    "type": "ERROR_TYPE",
    "errorMessage": "Human readable message"
  },
  "status": "error"
}
```

Known error types from webapp interceptors:
- `ACCOUNT_DELETED` → redirect to login
- `EMAIL_UNVERIFIED` → redirect to verify
