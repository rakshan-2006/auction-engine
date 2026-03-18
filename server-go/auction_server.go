package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var highestBid int = 0
var highestBidder string = ""
var mutex sync.Mutex
var clients = map[net.Conn]bool{}

func broadcast(message string) {
	for client := range clients {
		if _, err := client.Write([]byte(message)); err != nil {
			client.Close()
			delete(clients, client)
		}
	}
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

func handleConnection(conn net.Conn) {
	defer conn.Close()

	mutex.Lock()
	clients[conn] = true
	if highestBid > 0 {
		conn.Write([]byte(fmt.Sprintf("CURRENT_HIGHEST %s %d\n", highestBidder, highestBid)))
	}
	mutex.Unlock()

	defer func() {
		mutex.Lock()
		delete(clients, conn)
		mutex.Unlock()
	}()

	reader := bufio.NewReader(conn)

	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		message = strings.TrimSpace(message)
		parts := strings.Split(message, " ")
		if len(parts) < 3 || parts[0] != "BID" {
			conn.Write([]byte("INVALID_COMMAND\n"))
			continue
		}

		bidder := parts[1]
		bidAmount, err := strconv.Atoi(parts[2])
		if err != nil {
			conn.Write([]byte("INVALID_BID\n"))
			continue
		}

		mutex.Lock()

		if bidAmount > highestBid {

			highestBid = bidAmount
			highestBidder = bidder

			broadcast(fmt.Sprintf("NEW_HIGHEST %s %d\n", bidder, bidAmount))

		} else {

			conn.Write([]byte(fmt.Sprintf("BID_REJECTED HIGHEST %s %d\n", highestBidder, highestBid)))

		}

		mutex.Unlock()
	}
}

func main() {

	cert, _ := tls.LoadX509KeyPair("../ssl/server.crt", "../ssl/server.key")

	config := &tls.Config{Certificates: []tls.Certificate{cert}}

	listener, err := tls.Listen("tcp", ":8080", config)

	if err != nil {
		panic(err)
	}

	fmt.Println("Auction Server Started on port 8080")
	fmt.Println("Use one of these addresses from client devices:")
	fmt.Println("localhost:8080")
	for _, ip := range getLocalIPv4Addresses() {
		fmt.Printf("%s:8080\n", ip)
	}

	for {

		conn, err := listener.Accept()

		if err != nil {
			continue
		}

		go handleConnection(conn)

	}
}
