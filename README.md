# 🔨 Auction Engine

> A real-time, multi-client auction system — Go server, Java TCP client, and browser UI. Supports concurrent bidders across a LAN with anti-sniping protection and crash-safe persistence.

---

## Quick Start

```bash
# 1. Start the server
cd server-go && go mod tidy && go run auction_server.go

# 2. Connect a Java client (new terminal)
cd client-java && javac AuctionClient.java && java AuctionClient

# 3. Open the browser UI
open http://localhost:8090
```

---

## Architecture

```
┌─────────────────────────────────────────────┐
│              Go Auction Server               │
│                                             │
│  TLS TCP :8080          HTTP/WS :8090       │
│       │                     │               │
│  Java clients          Browser UI           │
│  (AuctionClient.java)  (index.html)         │
└─────────────────────────────────────────────┘
         │                   │
  Logs to disk:         Serves static files
  auction_events.log    from ui-web/
  auction_state.json
```

Two listeners run simultaneously:

- **`:8080` — TLS TCP** for Java (and any custom socket) clients
- **`:8090` — HTTP + WebSocket** for the browser UI and REST state endpoint

---

## Project Structure

```
auction-engine/
├── server-go/
│   ├── auction_server.go        # Full server implementation
│   ├── go.mod                   # Go 1.22+, gorilla/websocket
│   └── logs/
│       ├── auction_events.log   # Append-only JSON event log
│       └── auction_state.json   # Durable state snapshot
├── client-java/
│   └── AuctionClient.java       # Interactive TLS TCP bidding client
├── ui-web/
│   ├── index.html               # Bidding form + live dashboard
│   ├── style.css
│   └── app.js                   # WebSocket client logic
├── analytics-python/
│   └── monitor.py               # Local bid logger (manual input)
└── ssl/
    ├── server.crt               # Self-signed TLS certificate
    └── server.key
```

---

## Features

### Anti-Sniping

Any bid placed in the final 15 seconds extends the auction by 20 seconds — up to 6 times (max +120s total). No last-second steals.

| Parameter         | Value       |
|-------------------|-------------|
| Default duration  | 3 minutes   |
| Anti-snipe window | 15 seconds  |
| Extension per bid | +20 seconds |
| Max extensions    | 6           |

### Crash Recovery

State is atomically written to `auction_state.json` on every change (via temp-file rename). On restart, the server resumes any auction that hasn't yet ended.

### Consistent State

A background goroutine runs every 2 seconds to finalize winners, repair edge cases, and push a full `STATE` sync to all WebSocket clients.

### Bidder Validation

Names must be 2–32 characters — letters, digits, `_`, and `-` only.

### REST Endpoint

```
GET http://<host>:8090/api/state
```

Returns current auction state as JSON. Useful for external dashboards or health checks.

---

## Running the System

### Server (Go 1.22+)

```bash
cd server-go
go mod tidy
go run auction_server.go
```

Output includes all detected LAN IPs so other devices can connect easily:

```
Auction Server Started on TLS :8080
Auction UI Server Started on HTTP :8090
TLS clients: 192.168.x.x:8080
Browser UI:  http://192.168.x.x:8090
```

### Java Client (JDK 8+)

```bash
cd client-java
javac AuctionClient.java
java AuctionClient
```

```
Enter server IP or hostname (leave blank for localhost):
> [Enter for localhost, or LAN IP for remote]

Enter bidder name: alice
Enter bid amount: 100
Server: BID_ACCEPTED alice 100 1713500000000

Enter bid amount: 50
Server: BID_REJECTED HIGHEST alice 100
```

Run multiple clients simultaneously from different machines using the server's LAN IP.

### Browser UI

- Local: `http://localhost:8090`
- LAN: `http://<server-lan-ip>:8090`

Shows live leader, highest bid, countdown, extension count, and bid feed. Auto-reconnects on WebSocket drop.

### Python Monitor (optional)

```bash
cd analytics-python
python monitor.py
```

Standalone local logger — enter bids manually to track them with timestamps. Does **not** connect to the server.

---

## Protocol Reference

### TCP — Java / Custom Clients

**Send:**
```
BID <bidderName> <amount>\n
```

**Receive:**

| Message | Trigger |
|---|---|
| `CURRENT_HIGHEST <bidder\|NONE> <amount> <remainingMs>` | On connect |
| `BID_ACCEPTED <winner> <amount> <auctionEndUnixMs>` | Bid accepted |
| `NEW_HIGHEST <winner> <amount> <auctionEndUnixMs>` | Broadcast to all others |
| `BID_REJECTED HIGHEST <bidder> <amount>` | Bid too low |
| `BID_REJECTED <reason>` | Invalid name or amount |
| `AUCTION_CLOSED <winner> <amount>` | Auction ended |
| `INVALID_COMMAND` | Unrecognised command |
| `INVALID_BID` | Amount not a number |

### WebSocket — Browser UI

**Send (JSON):**
```json
{ "type": "BID", "bidder": "alice", "amount": 500 }
```

**Receive (JSON):**

| `type` | Key fields |
|---|---|
| `STATE` | `auctionId`, `highestBid`, `highestBidder`, `remainingMs`, `endTime`, `extensionCount`, `auctionFinalized` |
| `NEW_HIGHEST` | `bidder`, `amount`, `auctionEndsAt` |
| `BID_ACCEPTED` | `highestBid`, `highestBidder`, `auctionEndsAt` |
| `BID_REJECTED` | `decision`, `message`, `highestBid`, `highestBidder`, `auctionEndsAt` |
| `AUCTION_CLOSED` | `winner`, `amount` |
| `ERROR` | `message` |

---

## Event Log

Each line in `server-go/logs/auction_events.log` is a JSON object:

```json
{"type":"AUCTION_STARTED","auctionEndsAt":"2024-04-19T10:03:00Z","reason":"startup"}
{"type":"ACCEPTED","bidder":"alice","amount":100,"highestBid":100,"highestBidder":"alice","auctionEndsAt":"2024-04-19T10:03:00Z","serverReceivedAt":"2024-04-19T10:00:05Z"}
{"type":"REJECTED_LOWER_OR_EQUAL","bidder":"bob","amount":50,...}
{"type":"AUCTION_CLOSED","bidder":"alice","amount":100,...}
```

Possible `type` values: `AUCTION_STARTED` · `ACCEPTED` · `REJECTED_LOWER_OR_EQUAL` · `REJECTED_AUCTION_CLOSED` · `REJECTED_INVALID_BIDDER` · `REJECTED_INVALID_AMOUNT` · `AUCTION_CLOSED`

---

## Prerequisites

| Component      | Requirement            |
|----------------|------------------------|
| Server         | Go 1.22+               |
| Java client    | JDK 8+                 |
| Browser UI     | Any modern browser     |
| Python monitor | Python 3 (stdlib only) |

---

## Security

The Java client uses a trust-all TLS manager that accepts any certificate — intentional for local demo use with the bundled self-signed cert. **Do not use this in production.** Replace with proper certificate validation and a CA-signed certificate.

---

## Authors

- Om Pattanayak
- Rakshan R
- Ojas Taori
