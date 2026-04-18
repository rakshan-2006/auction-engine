# Auction Engine

Auction Engine is now a real-time, multi-device auction system with:

- time-critical bid acceptance
- fairness guarantees for near-closing bids
- thread-safe concurrent state updates
- failure-safe state persistence and consistency checks
- live browser UI plus existing Java TLS clients

## Core Features

### Time-critical bid handling

- The server stamps each incoming bid with server receive time.
- Bids are accepted only if they arrive before auction close time.
- Live countdown is available to browser clients and included in TCP responses.
- Anti-sniping extension: bids inside the last 15 seconds extend auction end by 20 seconds (up to 6 times).

### Fairness guarantees

- Single authoritative ordering by server-side sequence numbers.
- Equal or lower bids are deterministically rejected.
- Final winner is based on the highest accepted bid before close.

### Concurrent state updates

- Shared auction state is guarded by a mutex.
- Broadcast fan-out to Java TCP clients and WebSocket UI clients is done safely.
- Periodic state sync keeps all web clients aligned.

### Failure handling and consistency checks

- Every event is appended as JSON in `server-go/logs/auction_events.log`.
- Latest durable state snapshot is stored in `server-go/logs/auction_state.json`.
- Startup recovery restores previous state from disk.
- Background consistency loop repairs invalid edge states and finalizes auction when time expires.

## Architecture

- TLS socket server for Java and device clients: `:8080`
- HTTP + WebSocket server for UI: `:8090`
- Existing multi-device approach remains intact: any device in LAN can connect using host IP.

## Project Layout

```text
auction-engine/
  server-go/
    auction_server.go
    go.mod
    logs/
  client-java/
    AuctionClient.java
  ui-web/
    index.html
    style.css
    app.js
  analytics-python/
    monitor.py
  ssl/
    server.crt
    server.key
```

## Protocol Messages

### TCP (Java and custom socket clients)

Incoming:

```text
BID <name> <amount>
```

Outgoing examples:

- `CURRENT_HIGHEST <name-or-NONE> <amount> <remainingMs>`
- `NEW_HIGHEST <name> <amount> <auctionEndUnixMs>`
- `BID_ACCEPTED <name> <amount> <auctionEndUnixMs>`
- `BID_REJECTED HIGHEST <name> <amount>`
- `BID_REJECTED <reason>`
- `AUCTION_CLOSED <winner> <amount>`
- `INVALID_COMMAND`
- `INVALID_BID`

### WebSocket (Browser UI)

- `STATE`
- `NEW_HIGHEST`
- `BID_ACCEPTED`
- `BID_REJECTED`
- `AUCTION_CLOSED`

## Run Instructions

Open separate terminals for server and clients.

### 1) Start the server (Go)

```bash
cd server-go
go mod tidy
go run auction_server.go
```

You should see both listeners:

```text
Auction Server Started on TLS :8080
Auction UI Server Started on HTTP :8090
```

### 2) Start Java clients (existing multi-device flow)

```bash
cd client-java
javac AuctionClient.java
java AuctionClient
```

- Enter host/IP of server device (blank uses localhost).
- Enter bidder name.
- Enter amounts continuously.

### 3) Open browser UI

On same machine:

- `http://localhost:8090`

On other devices in same network:

- `http://<server-lan-ip>:8090`

### 4) Optional monitor script

```bash
cd analytics-python
python monitor.py
```

`monitor.py` remains a manual local logger.

## Security Note

- Java client currently trusts all certificates for demo simplicity. This is fine for local testing but not production.

## Authors

- Om Pattanayak
- Rakshan R
- Ojas Taori
