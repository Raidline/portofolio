package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"
)

// Ensures gofmt doesn't remove the "net" import in stage 1 (feel free to remove this!)
var _ = net.ListenUDP

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("Logs from your program will appear here!")

	fmt.Println(os.Args)
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: time.Millisecond * time.Duration(10000),
			}
			return d.DialContext(ctx, network, os.Args[2])
		},
	}

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:2053")
	if err != nil {
		fmt.Println("Failed to resolve UDP address:", err)
		return
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println("Failed to bind to address:", err)
		return
	}
	defer udpConn.Close()

	buf := make([]byte, 512)

	for {
		size, source, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			fmt.Println("Error receiving data:", err)
			break
		}

		receivedData := string(buf[:size])
		fmt.Printf("Received %d bytes from %s: %s\n", size, source, receivedData)

		message, messageErr := Unmarshall(buf, size)

		if messageErr != nil {
			fmt.Println("Error while parsing message:", messageErr)
			break
		}

		iterator := message.Transform(func(q dnsQuestion, a dnsAnswer) (dnsQuestion, dnsAnswer) {
			ips, err := resolver.LookupIP(context.Background(), "ip4", q.Name)
			if err != nil {
				fmt.Printf("Error resolving %s: %s\n", q.Name, err)
			}
			fmt.Printf("Resolved %s to %s\n", q.Name, ips[0])
			data := ips[0].To4()

			return dnsQuestion{
					Name:  q.Name,
					Type:  q.Class,
					Class: q.Class,
				}, dnsAnswer{
					Name:     a.Name,
					Type:     a.Type,
					Class:    a.Class,
					TTL:      a.TTL,
					RdLength: uint16(len(data)),
					Rdata:    data,
				}
		})
		for {
			ok := iterator()
			if !ok {
				break
			}
		}

		marshall, mErr := message.Marshall()

		if mErr != nil {
			fmt.Println("Failed to send header:", mErr)
			continue
		}

		_, err = udpConn.WriteToUDP(marshall, source)
		if err != nil {
			fmt.Println("Failed to send header:", err)
		}
	}
}
