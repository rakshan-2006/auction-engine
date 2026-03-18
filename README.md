# Auction Engine

This repository contains a small auction system built with sockets and TLS.

- Go server accepts bids over TLS on port 8080.
- Java clients connect, send bids, and receive live highest-bid updates.
- Python script is a simple manual logger for bid values and timestamps.

## What It Does

The server keeps two global values in memory:

- current highest bid amount
- name of the current highest bidder

Each client sends lines in this format:

```text
BID <name> <amount>
```

When a new highest bid arrives, the server broadcasts that update to every connected client.

## Project Layout

```text
auction-engine/
        server-go/
                auction_server.go
        client-java/
                AuctionClient.java
        analytics-python/
                monitor.py
        ssl/
                server.crt
                server.key
```

## Protocol Messages

Server responses currently used by the code:

- NEW_HIGHEST <name> <amount>
        - Sent to all connected clients when a higher bid is accepted.
- CURRENT_HIGHEST <name> <amount>
        - Sent to a client when it connects and a highest bid already exists.
- BID_REJECTED HIGHEST <name> <amount>
        - Sent to the bidder when bid is not higher than current highest.
- INVALID_COMMAND
        - Sent when incoming line does not match expected BID format.
- INVALID_BID
        - Sent when amount is not a valid integer.

## TLS and Certificates

The server runs with TLS and loads cert files from the ssl folder.

If you need to generate new self-signed files:

```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes
```

Run this command inside the ssl folder so files are created in the expected location.

## How to Run

Open separate terminals for server and each client.

### 1) Start server

```bash
cd server-go
go run auction_server.go
```

You should see:

```text
Auction Server Started on port 8080
```

### 2) Start one or more clients

```bash
cd client-java
javac AuctionClient.java
java AuctionClient
```

For each client:

- Enter server host (or leave blank for localhost).
- Enter bidder name.
- Enter bid amounts.

Each client has a background listener thread, so all clients print broadcast updates when a new highest bid is accepted.

### 3) Optional: run monitor script

```bash
cd analytics-python
python monitor.py
```

Important: monitor.py does not connect to the server.
It only logs values typed into its own terminal with timestamps.

## Example Flow

```text
Client A bids 100
Server broadcasts: NEW_HIGHEST Alice 100

Client B bids 150
Server broadcasts: NEW_HIGHEST Bob 150

Client A bids 120
Server replies to A: BID_REJECTED HIGHEST Bob 150
```

## Current Limitations

- Auction state is in-memory only (no database or persistence).
- No authentication; bidder name is plain text from client input.
- Java client currently trusts all certificates (good for demo, not for production).
- monitor.py is not integrated with network events.

## Authors

- Om Pattanayak
- Rakshan R
- Ojas Taori
