# Online Auction Engine (Real-Time Bidding)

## Project Overview

This project implements a **real-time online auction platform** using **low-level TCP socket programming**.
Multiple bidders (clients) can connect to a central auction server and place bids on items in real time.

The system ensures:

* **Secure communication using TLS/SSL**
* **Concurrent client handling**
* **Fair bidding rules**
* **Real-time updates of the highest bid**
* **Performance monitoring**

The project demonstrates the use of **network programming, concurrency, protocol design, and secure communication**.

---

# Technologies Used

### Programming Languages

* **Go** – Auction Server
* **Java** – Bidder Client
* **Python** – Monitoring and Analytics

### Networking

* TCP Socket Programming
* TLS/SSL Secure Communication

---

# System Architecture

The system follows a **client-server architecture**.

Java clients connect to the Go server using **TLS-secured TCP sockets**.
The server processes bids and maintains the current highest bid.
A Python monitoring script records auction activity.

```
                +-------------------+
                |   Java Client 1   |
                +-------------------+
                         |
                +-------------------+
                |   Java Client 2   |
                +-------------------+
                         |
                         v
                 +------------------+
                 |   Go Auction     |
                 |      Server      |
                 +------------------+
                         |
                         v
                +-------------------+
                |  Python Monitor   |
                |  (Analytics)      |
                +-------------------+
```

---

# Features Implemented

* Real-time bidding system
* Multiple concurrent bidders
* TLS-secured communication
* Bid validation and fairness rules
* Automatic rejection of lower bids
* Real-time highest bid updates
* Performance monitoring using Python

---

# Project Structure

```
auction-engine
│
├── server-go
│   └── auction_server.go
│
├── client-java
│   └── AuctionClient.java
│
├── analytics-python
│   └── monitor.py
│
├── ssl
│   ├── server.crt
│   └── server.key
│
└── README.md
```

---

# How to Run the Project

## Step 1 – Start the Auction Server

Open terminal and run:

```
cd server-go
go run auction_server.go
```

Expected output:

```
Auction Server Started on port 8080
```

---

## Step 2 – Start the Java Client

Open another terminal:

```
cd client-java
javac AuctionClient.java
java AuctionClient
```

Example interaction:

```
Enter bidder name:
Alice

Enter bid amount:
100

Server: NEW_HIGHEST Alice 100
```

---

## Step 3 – Start the Monitoring Script

Open another terminal:

```
cd analytics-python
python3 monitor.py
```

Example output:

```
Auction Monitor Started
Enter bid amount for logging:
```

---

# Example Auction Flow

Client 1:

```
Alice → 100
Server → NEW_HIGHEST Alice 100
```

Client 2:

```
Bob → 150
Server → NEW_HIGHEST Bob 150
```

Client 1 tries lower bid:

```
Alice → 120
Server → BID_REJECTED
```

---

# Performance Testing

The system was tested with multiple concurrent clients.

Test configuration:

* Number of clients: 3–5
* Concurrent bidding events
* Secure TLS communication enabled

Results:

* Average response time ≈ 10–15 ms
* Server handled multiple clients without data inconsistency
* Bid validation correctly enforced auction rules

---

# Failure Handling

The system handles several edge cases:

* Client disconnection
* Invalid bid values
* Lower bids than current highest bid
* Concurrent bid submissions

Example:

```
Current highest bid = 200
Client sends = 150
Server response = BID_REJECTED
```

---

# Security Implementation

All communication between clients and the server is secured using **TLS encryption**.

Self-signed certificates are generated using:

```
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes
```

This ensures encrypted communication between auction participants and the server.

---

# Conclusion

This project demonstrates the design and implementation of a **secure real-time auction platform** using **socket programming and concurrent server architecture**.

The system successfully supports:

* multiple bidders
* secure communication
* real-time bidding updates
* monitoring and performance evaluation.

---

# Author

Om Pattanayak
Rakshan R
Ojas Taori
