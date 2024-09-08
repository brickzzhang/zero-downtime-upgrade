package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

const (
	BusinessUdsAddr = "/tmp/socket_takeover.sock"
)

func main() {
	conn, err := net.Dial("unix", BusinessUdsAddr)
	if err != nil {
		panic(err)
	}

	for i := 0; ; i++ {
		conn.SetDeadline(time.Now().Local().Add(10 * time.Second))
		if _, err := conn.Write([]byte(fmt.Sprintf("hello server: %d", i))); err != nil {
			log.Print(err)
			return
		}
		var buf = make([]byte, 1024)
		if _, err := conn.Read(buf); err != nil {
			log.Printf("client read err: %s", err)
			continue
		}
		fmt.Println("client recv: ", string(buf))

		time.Sleep(10 * time.Millisecond)
	}
}
