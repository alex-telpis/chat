package chat

import (
	"bufio"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

type cluster struct {
	sync.Mutex

	Insts      map[string]*serverInst
	OwnAddr    string
	GuideAddr  string
	OtherAddrs []string
}

type serverInst struct {
	Addr   string
	Conn   net.Conn
	Reader *bufio.Reader

	cmdResp chan string
}

func (cl *cluster) Enabled() bool {
	return cl.GuideAddr != "" || len(cl.OtherAddrs) > 0
}

// Adds instance to cluster
func (cl *cluster) scaleUp(serv *serverInst) {
	cl.Lock()
	defer cl.Unlock()

	cl.OtherAddrs = append(cl.OtherAddrs, serv.Addr)
	cl.Insts[serv.Addr] = serv
}

// Returns cluster members addresses except own addr and requester's addr
func (cl *cluster) shareMembers(rAddr string) []string {
	res := []string{}
	for addr, _ := range cl.Insts {
		if addr != rAddr {
			res = append(res, addr)
		}
	}
	return res
}

// Checks online status of a user across the cluster
func (cl *cluster) Online(username string) bool {
	cl.Lock()
	defer cl.Unlock()

	//	var wg sync.WaitGroup

	for _, serv := range cl.Insts {
		resp, err := serv.commandRequest(prefixOnlineStatusReq+username+"\n", 2*time.Second)
		if err != nil {
			continue
		}

		if strings.Contains(resp, "yes") {
			return true
		}
	}

	return false
}

// Sends a request (e.g. check online status) to a cluster instance
// Returns responce
func (serv *serverInst) commandRequest(msg string, tm time.Duration) (string, error) {
	serv.Conn.Write([]byte(msg))
	timeout := time.After(tm)

	select {
	case res := <-serv.cmdResp:
		return res, nil
	case <-timeout:
		return "", errors.New("timedout")
	}

	return "", nil
}
