package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
)

var highestBid int = 0
var highestBidder string = ""
var mutex sync.Mutex

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

	for {

		conn, err := listener.Accept()

		if err != nil {
			continue
		}

		go handleConnection(conn)

	}
}
