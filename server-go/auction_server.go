package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	tcpListenAddr   = ":8080"
	httpListenAddr  = ":8090"
	defaultDuration = 3 * time.Minute
	extendWindow    = 15 * time.Second
	extendBy        = 20 * time.Second
	maxExtensions   = 6
)

type BidDecision string

const (
	DecisionAccepted      BidDecision = "ACCEPTED"
	DecisionRejectedLow   BidDecision = "REJECTED_LOWER_OR_EQUAL"
	DecisionRejectedLate  BidDecision = "REJECTED_AUCTION_CLOSED"
	DecisionRejectedName  BidDecision = "REJECTED_INVALID_BIDDER"
	DecisionRejectedValue BidDecision = "REJECTED_INVALID_AMOUNT"
)

type BidResult struct {
	Decision         BidDecision `json:"decision"`
	Bidder           string      `json:"bidder"`
	Amount           int         `json:"amount"`
	HighestBid       int         `json:"highestBid"`
	HighestBidder    string      `json:"highestBidder"`
	AuctionEndsAt    time.Time   `json:"auctionEndsAt"`
	ServerReceivedAt time.Time   `json:"serverReceivedAt"`
	Message          string      `json:"message"`
}

type BidEvent struct {
	Type             string    `json:"type"`
	Bidder           string    `json:"bidder,omitempty"`
	Amount           int       `json:"amount,omitempty"`
	HighestBid       int       `json:"highestBid,omitempty"`
	HighestBidder    string    `json:"highestBidder,omitempty"`
	AuctionEndsAt    time.Time `json:"auctionEndsAt,omitempty"`
	ServerReceivedAt time.Time `json:"serverReceivedAt,omitempty"`
	Reason           string    `json:"reason,omitempty"`
}

type PersistentState struct {
	AuctionID         string    `json:"auctionId"`
	StartTime         time.Time `json:"startTime"`
	EndTime           time.Time `json:"endTime"`
	HighestBid        int       `json:"highestBid"`
	HighestBidder     string    `json:"highestBidder"`
	HighestBidSeq     uint64    `json:"highestBidSeq"`
	ExtensionCount    int       `json:"extensionCount"`
	AuctionFinalized  bool      `json:"auctionFinalized"`
	FinalizedAt       time.Time `json:"finalizedAt,omitempty"`
	LastUpdated       time.Time `json:"lastUpdated"`
	WinnerAnnouncedAt time.Time `json:"winnerAnnouncedAt,omitempty"`
}

type AuctionState struct {
	mu sync.Mutex

	auctionID        string
	startTime        time.Time
	endTime          time.Time
	highestBid       int
	highestBidder    string
	highestBidSeq    uint64
	extensionCount   int
	auctionFinalized bool
	finalizedAt      time.Time

	bidSeqCounter uint64
	tcpClients    map[net.Conn]bool
	wsClients     map[*websocket.Conn]bool

	eventLogPath string
	statePath    string
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func newAuctionState(baseDir string) *AuctionState {
	_ = os.MkdirAll(filepath.Join(baseDir, "logs"), 0o755)

	now := time.Now().UTC()
	state := &AuctionState{
		auctionID:      now.Format("20060102-150405"),
		startTime:      now,
		endTime:        now.Add(defaultDuration),
		tcpClients:     make(map[net.Conn]bool),
		wsClients:      make(map[*websocket.Conn]bool),
		eventLogPath:   filepath.Join(baseDir, "logs", "auction_events.log"),
		statePath:      filepath.Join(baseDir, "logs", "auction_state.json"),
		bidSeqCounter:  0,
		highestBidSeq:  0,
		extensionCount: 0,
	}

	state.restoreFromDisk()
	state.persistStateLocked(time.Now().UTC())
	state.appendEvent(BidEvent{
		Type:          "AUCTION_STARTED",
		AuctionEndsAt: state.endTime,
		Reason:        "startup",
	})

	return state
}

func (a *AuctionState) restoreFromDisk() {
	a.mu.Lock()
	defer a.mu.Unlock()

	content, err := os.ReadFile(a.statePath)
	if err != nil {
		return
	}

	var saved PersistentState
	if err := json.Unmarshal(content, &saved); err != nil {
		log.Printf("state restore skipped: %v", err)
		return
	}

	now := time.Now().UTC()
	if saved.AuctionFinalized || saved.EndTime.Before(now) {
		log.Printf("state restore skipped: saved auction is already closed")
		return
	}

	a.auctionID = saved.AuctionID
	a.startTime = saved.StartTime
	a.endTime = saved.EndTime
	a.highestBid = saved.HighestBid
	a.highestBidder = saved.HighestBidder
	a.highestBidSeq = saved.HighestBidSeq
	a.bidSeqCounter = saved.HighestBidSeq
	a.extensionCount = saved.ExtensionCount
	a.auctionFinalized = saved.AuctionFinalized
	a.finalizedAt = saved.FinalizedAt
}

func (a *AuctionState) snapshotLocked(now time.Time) PersistentState {
	return PersistentState{
		AuctionID:        a.auctionID,
		StartTime:        a.startTime,
		EndTime:          a.endTime,
		HighestBid:       a.highestBid,
		HighestBidder:    a.highestBidder,
		HighestBidSeq:    a.highestBidSeq,
		ExtensionCount:   a.extensionCount,
		AuctionFinalized: a.auctionFinalized,
		FinalizedAt:      a.finalizedAt,
		LastUpdated:      now,
	}
}

func (a *AuctionState) persistStateLocked(now time.Time) {
	snapshot := a.snapshotLocked(now)
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		log.Printf("state snapshot marshal error: %v", err)
		return
	}

	tmpPath := a.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, encoded, 0o644); err != nil {
		log.Printf("state snapshot write error: %v", err)
		return
	}

	if err := os.Rename(tmpPath, a.statePath); err != nil {
		log.Printf("state snapshot replace error: %v", err)
	}
}

func (a *AuctionState) appendEvent(event BidEvent) {
	eventLine, err := json.Marshal(event)
	if err != nil {
		log.Printf("event marshal error: %v", err)
		return
	}

	f, err := os.OpenFile(a.eventLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("event log open error: %v", err)
		return
	}
	defer f.Close()

	if _, err := f.Write(append(eventLine, '\n')); err != nil {
		log.Printf("event log write error: %v", err)
	}
}

func isValidBidderName(name string) bool {
	if len(name) < 2 || len(name) > 32 {
		return false
	}

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}

	return true
}

func (a *AuctionState) timeRemainingLocked(now time.Time) time.Duration {
	if now.After(a.endTime) {
		return 0
	}
	return a.endTime.Sub(now)
}

func (a *AuctionState) stateMessageLocked(prefix string) string {
	remainingMs := a.timeRemainingLocked(time.Now().UTC()).Milliseconds()
	if remainingMs < 0 {
		remainingMs = 0
	}

	if a.highestBid == 0 || a.highestBidder == "" {
		return fmt.Sprintf("%s NONE 0 %d\n", prefix, remainingMs)
	}

	return fmt.Sprintf("%s %s %d %d\n", prefix, a.highestBidder, a.highestBid, remainingMs)
}

func (a *AuctionState) addTCPClient(conn net.Conn) {
	a.mu.Lock()
	a.tcpClients[conn] = true
	message := a.stateMessageLocked("CURRENT_HIGHEST")
	a.mu.Unlock()

	_, _ = conn.Write([]byte(message))
}

func (a *AuctionState) removeTCPClient(conn net.Conn) {
	a.mu.Lock()
	delete(a.tcpClients, conn)
	a.mu.Unlock()
}

func (a *AuctionState) addWSClient(conn *websocket.Conn) {
	a.mu.Lock()
	a.wsClients[conn] = true
	state := a.wsStatePayloadLocked(time.Now().UTC())
	a.mu.Unlock()

	_ = conn.WriteJSON(state)
}

func (a *AuctionState) removeWSClient(conn *websocket.Conn) {
	a.mu.Lock()
	delete(a.wsClients, conn)
	a.mu.Unlock()
}

func (a *AuctionState) wsStatePayloadLocked(now time.Time) map[string]any {
	remainingMs := a.timeRemainingLocked(now).Milliseconds()
	if remainingMs < 0 {
		remainingMs = 0
	}

	return map[string]any{
		"type":             "STATE",
		"auctionId":        a.auctionID,
		"highestBid":       a.highestBid,
		"highestBidder":    a.highestBidder,
		"remainingMs":      remainingMs,
		"endTime":          a.endTime,
		"extensionCount":   a.extensionCount,
		"auctionFinalized": a.auctionFinalized,
	}
}

func (a *AuctionState) processBid(bidder string, amount int, serverReceivedAt time.Time) BidResult {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !isValidBidderName(bidder) {
		return BidResult{
			Decision:         DecisionRejectedName,
			Bidder:           bidder,
			Amount:           amount,
			HighestBid:       a.highestBid,
			HighestBidder:    a.highestBidder,
			AuctionEndsAt:    a.endTime,
			ServerReceivedAt: serverReceivedAt,
			Message:          "Bidder name must be 2-32 chars and use letters, numbers, _ or -",
		}
	}

	if amount <= 0 {
		return BidResult{
			Decision:         DecisionRejectedValue,
			Bidder:           bidder,
			Amount:           amount,
			HighestBid:       a.highestBid,
			HighestBidder:    a.highestBidder,
			AuctionEndsAt:    a.endTime,
			ServerReceivedAt: serverReceivedAt,
			Message:          "Bid amount must be positive",
		}
	}

	if a.auctionFinalized || serverReceivedAt.After(a.endTime) {
		a.auctionFinalized = true
		if a.finalizedAt.IsZero() {
			a.finalizedAt = serverReceivedAt
		}
		a.persistStateLocked(serverReceivedAt)

		return BidResult{
			Decision:         DecisionRejectedLate,
			Bidder:           bidder,
			Amount:           amount,
			HighestBid:       a.highestBid,
			HighestBidder:    a.highestBidder,
			AuctionEndsAt:    a.endTime,
			ServerReceivedAt: serverReceivedAt,
			Message:          "Auction already closed",
		}
	}

	bidSeq := atomic.AddUint64(&a.bidSeqCounter, 1)
	if amount > a.highestBid {
		a.highestBid = amount
		a.highestBidder = bidder
		a.highestBidSeq = bidSeq

		if a.extensionCount < maxExtensions && a.endTime.Sub(serverReceivedAt) <= extendWindow {
			a.endTime = a.endTime.Add(extendBy)
			a.extensionCount++
		}

		a.persistStateLocked(serverReceivedAt)

		return BidResult{
			Decision:         DecisionAccepted,
			Bidder:           bidder,
			Amount:           amount,
			HighestBid:       a.highestBid,
			HighestBidder:    a.highestBidder,
			AuctionEndsAt:    a.endTime,
			ServerReceivedAt: serverReceivedAt,
			Message:          "Bid accepted",
		}
	}

	return BidResult{
		Decision:         DecisionRejectedLow,
		Bidder:           bidder,
		Amount:           amount,
		HighestBid:       a.highestBid,
		HighestBidder:    a.highestBidder,
		AuctionEndsAt:    a.endTime,
		ServerReceivedAt: serverReceivedAt,
		Message:          "Bid is not higher than current highest",
	}
}

func (a *AuctionState) broadcastAccepted(result BidResult) {
	a.mu.Lock()
	tcpClients := make([]net.Conn, 0, len(a.tcpClients))
	for conn := range a.tcpClients {
		tcpClients = append(tcpClients, conn)
	}

	wsClients := make([]*websocket.Conn, 0, len(a.wsClients))
	for conn := range a.wsClients {
		wsClients = append(wsClients, conn)
	}

	tcpMessage := fmt.Sprintf("NEW_HIGHEST %s %d %d\n", result.HighestBidder, result.HighestBid, result.AuctionEndsAt.UnixMilli())
	wsEvent := map[string]any{
		"type":             "NEW_HIGHEST",
		"bidder":           result.HighestBidder,
		"amount":           result.HighestBid,
		"serverReceivedAt": result.ServerReceivedAt,
		"auctionEndsAt":    result.AuctionEndsAt,
		"decision":         result.Decision,
		"message":          result.Message,
	}
	a.mu.Unlock()

	for _, conn := range tcpClients {
		if _, err := conn.Write([]byte(tcpMessage)); err != nil {
			_ = conn.Close()
			a.removeTCPClient(conn)
		}
	}

	for _, conn := range wsClients {
		if err := conn.WriteJSON(wsEvent); err != nil {
			_ = conn.Close()
			a.removeWSClient(conn)
		}
	}

	a.appendEvent(BidEvent{
		Type:             "BID_ACCEPTED",
		Bidder:           result.Bidder,
		Amount:           result.Amount,
		HighestBid:       result.HighestBid,
		HighestBidder:    result.HighestBidder,
		AuctionEndsAt:    result.AuctionEndsAt,
		ServerReceivedAt: result.ServerReceivedAt,
	})
}

func (a *AuctionState) sendWSStateToAll() {
	a.mu.Lock()
	wsClients := make([]*websocket.Conn, 0, len(a.wsClients))
	for conn := range a.wsClients {
		wsClients = append(wsClients, conn)
	}
	payload := a.wsStatePayloadLocked(time.Now().UTC())
	a.mu.Unlock()

	for _, conn := range wsClients {
		if err := conn.WriteJSON(payload); err != nil {
			_ = conn.Close()
			a.removeWSClient(conn)
		}
	}
}

func (a *AuctionState) maybeFinalizeAuction() {
	a.mu.Lock()
	now := time.Now().UTC()
	if a.auctionFinalized || now.Before(a.endTime) {
		a.mu.Unlock()
		return
	}

	a.auctionFinalized = true
	a.finalizedAt = now
	a.persistStateLocked(now)

	tcpClients := make([]net.Conn, 0, len(a.tcpClients))
	for conn := range a.tcpClients {
		tcpClients = append(tcpClients, conn)
	}
	wsClients := make([]*websocket.Conn, 0, len(a.wsClients))
	for conn := range a.wsClients {
		wsClients = append(wsClients, conn)
	}

	winner := a.highestBidder
	amount := a.highestBid
	a.mu.Unlock()

	tcpMsg := fmt.Sprintf("AUCTION_CLOSED %s %d\n", winner, amount)
	for _, conn := range tcpClients {
		if _, err := conn.Write([]byte(tcpMsg)); err != nil {
			_ = conn.Close()
			a.removeTCPClient(conn)
		}
	}

	for _, conn := range wsClients {
		if err := conn.WriteJSON(map[string]any{
			"type":   "AUCTION_CLOSED",
			"winner": winner,
			"amount": amount,
		}); err != nil {
			_ = conn.Close()
			a.removeWSClient(conn)
		}
	}

	a.appendEvent(BidEvent{
		Type:          "AUCTION_CLOSED",
		Bidder:        winner,
		Amount:        amount,
		HighestBid:    amount,
		HighestBidder: winner,
		AuctionEndsAt: now,
	})
}

func (a *AuctionState) consistencyCheckerLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		a.maybeFinalizeAuction()

		a.mu.Lock()
		now := time.Now().UTC()

		if a.highestBid == 0 && a.highestBidder != "" {
			log.Printf("consistency check: clearing bidder with zero highest bid")
			a.highestBidder = ""
			a.persistStateLocked(now)
		}

		if a.highestBid > 0 && a.highestBidder == "" {
			log.Printf("consistency check: highest bid has no bidder, resetting bid")
			a.highestBid = 0
			a.persistStateLocked(now)
		}

		if a.endTime.Before(a.startTime) {
			log.Printf("consistency check: fixing invalid auction timeline")
			a.endTime = a.startTime.Add(defaultDuration)
			a.persistStateLocked(now)
		}

		a.mu.Unlock()
		a.sendWSStateToAll()
	}
}

func (a *AuctionState) handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	a.addTCPClient(conn)
	defer a.removeTCPClient(conn)

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		message := strings.TrimSpace(line)
		parts := strings.Split(message, " ")
		if len(parts) < 3 || parts[0] != "BID" {
			_, _ = conn.Write([]byte("INVALID_COMMAND\n"))
			continue
		}

		bidder := strings.TrimSpace(parts[1])
		amount, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			_, _ = conn.Write([]byte("INVALID_BID\n"))
			continue
		}

		result := a.processBid(bidder, amount, time.Now().UTC())
		switch result.Decision {
		case DecisionAccepted:
			a.broadcastAccepted(result)
			_, _ = conn.Write([]byte(fmt.Sprintf("BID_ACCEPTED %s %d %d\n", result.HighestBidder, result.HighestBid, result.AuctionEndsAt.UnixMilli())))
		case DecisionRejectedLate:
			_, _ = conn.Write([]byte(fmt.Sprintf("AUCTION_CLOSED %s %d\n", result.HighestBidder, result.HighestBid)))
		case DecisionRejectedLow:
			_, _ = conn.Write([]byte(fmt.Sprintf("BID_REJECTED HIGHEST %s %d\n", result.HighestBidder, result.HighestBid)))
		case DecisionRejectedName, DecisionRejectedValue:
			_, _ = conn.Write([]byte(fmt.Sprintf("BID_REJECTED %s\n", result.Message)))
		default:
			_, _ = conn.Write([]byte("INVALID_BID\n"))
		}

		a.appendEvent(BidEvent{
			Type:             string(result.Decision),
			Bidder:           result.Bidder,
			Amount:           result.Amount,
			HighestBid:       result.HighestBid,
			HighestBidder:    result.HighestBidder,
			AuctionEndsAt:    result.AuctionEndsAt,
			ServerReceivedAt: result.ServerReceivedAt,
			Reason:           result.Message,
		})
	}
}

func (a *AuctionState) handleWSConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	a.addWSClient(conn)
	defer func() {
		a.removeWSClient(conn)
		_ = conn.Close()
	}()

	for {
		var incoming struct {
			Type   string `json:"type"`
			Bidder string `json:"bidder"`
			Amount int    `json:"amount"`
		}

		if err := conn.ReadJSON(&incoming); err != nil {
			return
		}

		if incoming.Type != "BID" {
			_ = conn.WriteJSON(map[string]any{"type": "ERROR", "message": "Unsupported message type"})
			continue
		}

		result := a.processBid(strings.TrimSpace(incoming.Bidder), incoming.Amount, time.Now().UTC())
		if result.Decision == DecisionAccepted {
			a.broadcastAccepted(result)
			_ = conn.WriteJSON(map[string]any{
				"type":          "BID_ACCEPTED",
				"highestBid":    result.HighestBid,
				"highestBidder": result.HighestBidder,
				"auctionEndsAt": result.AuctionEndsAt,
			})
		} else {
			_ = conn.WriteJSON(map[string]any{
				"type":          "BID_REJECTED",
				"decision":      result.Decision,
				"message":       result.Message,
				"highestBid":    result.HighestBid,
				"highestBidder": result.HighestBidder,
				"auctionEndsAt": result.AuctionEndsAt,
			})
		}

		a.appendEvent(BidEvent{
			Type:             string(result.Decision),
			Bidder:           result.Bidder,
			Amount:           result.Amount,
			HighestBid:       result.HighestBid,
			HighestBidder:    result.HighestBidder,
			AuctionEndsAt:    result.AuctionEndsAt,
			ServerReceivedAt: result.ServerReceivedAt,
			Reason:           result.Message,
		})
	}
}

func (a *AuctionState) handleHTTPState(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	state := a.wsStatePayloadLocked(time.Now().UTC())
	a.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}

func getLocalIPv4Addresses() []string {
	addresses := []string{}
	seen := map[string]bool{}

	interfaces, err := net.Interfaces()
	if err != nil {
		return addresses
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			ipStr := ip.String()
			if !seen[ipStr] {
				seen[ipStr] = true
				addresses = append(addresses, ipStr)
			}
		}
	}

	sort.Strings(addresses)
	return addresses
}

func main() {
	state := newAuctionState(".")
	go state.consistencyCheckerLoop()

	cert, err := tls.LoadX509KeyPair("../ssl/server.crt", "../ssl/server.key")
	if err != nil {
		log.Fatalf("unable to load TLS certs: %v", err)
	}

	config := &tls.Config{Certificates: []tls.Certificate{cert}}

	listener, err := tls.Listen("tcp", tcpListenAddr, config)
	if err != nil {
		log.Fatalf("unable to start TLS listener: %v", err)
	}
	defer listener.Close()

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/ws", state.handleWSConnection)
	httpMux.HandleFunc("/api/state", state.handleHTTPState)
	httpMux.Handle("/", http.FileServer(http.Dir("../ui-web")))

	go func() {
		log.Printf("UI and WebSocket endpoint available at http://localhost%s", httpListenAddr)
		if err := http.ListenAndServe(httpListenAddr, httpMux); err != nil {
			log.Fatalf("http server error: %v", err)
		}
	}()

	fmt.Printf("Auction Server Started on TLS %s\n", tcpListenAddr)
	fmt.Printf("Auction UI Server Started on HTTP %s\n", httpListenAddr)
	fmt.Println("Use one of these addresses from client devices:")
	fmt.Println("TLS clients: localhost:8080")
	fmt.Println("Browser UI:  http://localhost:8090")
	for _, ip := range getLocalIPv4Addresses() {
		fmt.Printf("TLS clients: %s:8080\n", ip)
		fmt.Printf("Browser UI:  http://%s:8090\n", ip)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go state.handleTCPConnection(conn)
	}
}
