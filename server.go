package chat

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	prefixHsClient         = "username:"
	prefixHsServer         = "addr:"
	prefixOnlineStatusReq  = "isonline:"
	prefixOnlineStatusResp = "onlinestatus:"
)

type server struct {
	sync.Mutex
	Users   map[string][]*User
	Cluster *cluster
}

func NewServer(ownAddr string, guideAddr string) *server {
	s := new(server)
	s.Users = map[string][]*User{}
	s.Cluster = &cluster{
		Insts:      map[string]*serverInst{},
		OwnAddr:    ownAddr,
		GuideAddr:  guideAddr,
		OtherAddrs: []string{},
	}

	return s
}

func (s *server) Run(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to start server: %s", err.Error())
	}

	log.Printf("Server started on port %s", port)

	if s.Cluster.Enabled() {

		log.Printf("cluster enabled. Requesting addresses from guide %s ...", s.Cluster.GuideAddr)
		addrs, err := s.joinGuideServer()
		if err != nil {
			log.Fatalf("Failed to connect to guide server: %s", err.Error())
		}

		if len(addrs) > 0 {
			s.Cluster.OtherAddrs = addrs
			log.Printf("joining cluster %v", s.Cluster.OtherAddrs)
			s.joinCluster()
		}
	}

	for {
		// accept connections
		conn, err := listener.Accept()
		if err != nil {
			log.Fatalf("Can't accept connection: %s", err.Error())
		}

		s.serveConnection(conn)
	}
}

// Connects to all cluster instances
func (s *server) joinCluster() {
	for _, addr := range s.Cluster.OtherAddrs {
		_, err := s.joinServer(addr)
		if err != nil {
			fmt.Printf("cluster: %s", err.Error())
			continue
		}
		log.Printf("cluster: paired with %s", addr)
	}
}

// Connects to a cluster instance
func (s *server) joinServer(addr string) (string, error) {
	inst := &serverInst{Addr: addr, cmdResp: make(chan string)}

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	inst.Conn = conn
	if err != nil {
		return "", errors.New(fmt.Sprintf("inst %s is not available", addr))
	}
	inst.Reader = bufio.NewReader(inst.Conn)

	// Send connection request
	resp, err := handshakeRequest(
		prefixHsServer+s.Cluster.OwnAddr+"\n",
		inst.Conn,
		3*time.Second,
	)
	if err != nil {
		return "", errors.New(fmt.Sprintf("inst %s failed to handshake: %s", addr, err.Error()))
	}

	s.Cluster.Insts[addr] = inst
	go s.listenServer(inst)

	return resp, nil
}

// Connects to "guide" server and obtains addresses of other cluster instances
func (s *server) joinGuideServer() ([]string, error) {
	addrs, err := s.joinServer(s.Cluster.GuideAddr)
	addrs = strings.Trim(addrs, "\n")
	if err != nil {
		return []string{}, err
	}

	res := []string{}
	for _, a := range strings.Split(addrs, ",") {
		if a != "" {
			res = append(res, a)
		}
	}

	return res, nil
}

// Handles connection (user or cluster inst)
func (s *server) serveConnection(conn net.Conn) {
	reader := bufio.NewReader(conn)

	timeout := time.After(3 * time.Second)
	u := make(chan *User, 1)
	i := make(chan *serverInst, 1)
	defer close(u)
	defer close(i)

	go func() {
		for {
			if handshake, _ := reader.ReadString('\n'); handshake != "" {
				if check, username := isClientHandshake(handshake); check {
					u <- &User{
						Name:   username,
						Conn:   conn,
						Reader: reader,
					}
				} else if check, addr := isServerHandshake(handshake); check {
					i <- &serverInst{
						Addr:    addr,
						Conn:    conn,
						Reader:  reader,
						cmdResp: make(chan string),
					}
				}

				return
			}
		}
	}()

	select {
	case user := <-u:
		user.Conn.Write([]byte("ok\n"))
		s.serveUser(user)
	case serv := <-i:
		// send cluster instances list in responce
		addrs := strings.Join(s.Cluster.shareMembers(serv.Addr), ",")
		s.sendToServer(addrs+"\n", serv)

		if _, connected := s.Cluster.Insts[serv.Addr]; !connected {
			log.Printf("cluster+ : accepted connection from %s", serv.Addr)
			s.Cluster.scaleUp(serv)
			go s.listenServer(serv)
		}
	case <-timeout:
		log.Printf("!!! connection timedout while handshake")
		return
	}
}

// Handles user activity
func (s *server) serveUser(user *User) {
	isNew := s.registerUser(user)
	if isNew && !s.Cluster.Online(user.Name) {
		s.send(fmt.Sprintf(">>> %s just arrived\n", user.Name))
	}

	// handle user chat
	clientChan := make(chan string) // messages from the client
	go s.listenUser(user, clientChan)
	go s.publishUserMessages(user, clientChan)
}

// Obtains requests from a cluster instance
func (s *server) listenServer(serv *serverInst) {
	for {
		msg, err := serv.Reader.ReadString('\n')
		if err != nil {
			// disconnect
			delete(s.Cluster.Insts, serv.Addr)
			log.Println("cluster- : lost connection with " + serv.Addr)
			return
		}
		if msg != "" {
			if match, username := isOnlineStatusRequest(msg); match {
				resp := prefixOnlineStatusResp + "no\n"
				if s.userOnline(username) {
					resp = prefixOnlineStatusResp + "yes\n"
				}
				s.sendToServer(resp, serv)
			} else if match, resp := isOnlineStatusResponce(msg); match {
				serv.cmdResp <- resp
			} else {
				s.sendToUsers(msg)
			}
		}
	}
}

// Receives messages from a user and pushes them into user-communication channel
func (s *server) listenUser(user *User, output chan string) {
	defer close(output)

	for {
		msg, err := user.Reader.ReadString('\n')
		if err != nil {
			// disconnect
			onlineLocal := s.removeUser(user)
			if !onlineLocal && !s.Cluster.Online(user.Name) {
				s.send(fmt.Sprintf("<<< %s disconneted\n", user.Name))
			}
			return
		}
		if msg != "" {
			output <- msg
		}
	}
}

// Streams messages from a specified user to other chat members
func (s *server) publishUserMessages(user *User, input chan string) {
	for msg := range input {
		s.send(user.Name + ": " + msg)
	}
}

// Sends a message to specified local user
func (s *server) sendToUser(msg string, user *User) {
	s.sendTo(msg, user.Conn)
}

// Sends a message to all local user
func (s *server) sendToUsers(msg string) {
	log.Print("publish: " + msg)
	var wg sync.WaitGroup

	for _, userClients := range s.Users {
		for _, user := range userClients {
			wg.Add(1)
			go func() {
				s.sendToUser(msg, user)
				wg.Done()
			}()
		}
	}
	wg.Wait()
}

// Sends a message to specified server
func (s *server) sendToServer(msg string, serv *serverInst) {
	s.sendTo(msg, serv.Conn)
}

func (s *server) sendToServers(msg string) {
	log.Printf("notify %d servers: %s", len(s.Cluster.Insts), msg)
	for _, serv := range s.Cluster.Insts {
		s.sendToServer(msg, serv)
	}
}

// Sends a message to a connection
func (s *server) sendTo(msg string, conn net.Conn) {
	conn.Write([]byte(msg))
}

// Sends a message to all users
func (s *server) send(msg string) {
	s.Lock()
	defer s.Unlock()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		s.sendToUsers(msg)
		wg.Done()
	}()

	if s.Cluster.Enabled() {
		wg.Add(1)
		go func() {
			s.sendToServers(msg)
			wg.Done()
		}()
	}
	wg.Wait()
}

// Adds new user, returns true if it is virst visit during session
func (s *server) registerUser(user *User) (first bool) {
	s.Lock()
	defer s.Unlock()

	if s.userOnline(user.Name) {
		s.Users[user.Name] = append(s.Users[user.Name], user)
		return false
	} else {
		s.Users[user.Name] = []*User{user}
		return true
	}
}

// Removes user from chat, returns true if the user is still online with other connection
func (s *server) removeUser(user *User) (online bool) {
	s.Lock()
	defer s.Unlock()

	if s.userOnline(user.Name) {
		refreshed := []*User{}
		for _, u := range s.Users[user.Name] {
			if u != user {
				refreshed = append(refreshed, u)
			}
		}

		if len(refreshed) == 0 {
			delete(s.Users, user.Name)
			return false
		} else {
			s.Users[user.Name] = refreshed
			return true
		}
	}

	return false
}

// Local online status
func (s *server) userOnline(name string) bool {
	if _, ok := s.Users[name]; ok && len(s.Users[name]) > 0 {
		return true
	}
	return false
}
