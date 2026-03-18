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

	reader := bufio.NewReader(conn)

	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		message = strings.TrimSpace(message)
		parts := strings.Split(message, " ")

		if parts[0] == "BID" {

			bidder := parts[1]
			bidAmount, _ := strconv.Atoi(parts[2])

			mutex.Lock()

			if bidAmount > highestBid {

				highestBid = bidAmount
				highestBidder = bidder

				response := fmt.Sprintf("NEW_HIGHEST %s %d\n", bidder, bidAmount)
				conn.Write([]byte(response))

			} else {

				conn.Write([]byte("BID_REJECTED\n"))

			}

			mutex.Unlock()
		}
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
