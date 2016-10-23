package chat

import (
	"bufio"
	"log"
	"net"
	"os"
	"time"
)

func NewClient(username string) *client {
	return &client{User{Name: username}}
}

type User struct {
	Name   string
	Conn   net.Conn
	Reader *bufio.Reader
}

type client struct {
	User
}

func (c *client) Run(serverAddr string) {
	log.Println("Run client")

	// open connection
	conn, err := net.DialTimeout("tcp", serverAddr, 3*time.Second)
	c.Conn = conn
	if err != nil {
		log.Fatalf("Connection refused: %s", err.Error())
	}
	defer c.Conn.Close()
	c.Reader = bufio.NewReader(c.Conn)

	// Send connection request
	log.Println("Connecting to server ...")
	_, err = handshakeRequest(prefixHsClient+c.Name+"\n", c.Conn, 3*time.Second)
	if err != nil {
		log.Fatalln(err.Error())
	}
	log.Println("[ok] connected!")

	// send messages
	c.handleInput()

	// read messages
	for {
		message, err := c.Reader.ReadString('\n')
		if err != nil {
			log.Fatalf("Connection lost")
		}

		if message != "" {
			log.Print(message)
		}
	}
}

// Scans CMD
func (c *client) handleInput() {
	go func() {
		reader := bufio.NewReader(os.Stdin)
		log.Print("Enter text: ")
		for {
			text, _ := reader.ReadString('\n')
			c.Conn.Write([]byte(text))
		}
	}()
}
