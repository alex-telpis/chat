package chat

import (
	"bufio"
	"errors"
	"net"
	"strings"
	"time"
)

func handshakeRequest(msg string, conn net.Conn, tm time.Duration) (string, error) {
	conn.Write([]byte(msg))
	r := bufio.NewReader(conn)

	timeout := time.After(tm)
	e := make(chan error, 1)
	confirm := make(chan string, 1)

	go func() {
		for {
			msg, err := r.ReadString('\n')
			if err != nil {
				e <- err
				return
			}

			if msg != "" {
				confirm <- msg
				return
			}
		}
	}()

	select {
	case resp := <-confirm:
		return resp, nil
	case <-timeout:
		return "", errors.New("timedout")
	case err := <-e:
		return "", err
	}

	return "", nil
}

// Returns true if string matches prefix
// string without prefix returned as 2nd value
func parseByPrefix(str, prefix string) (bool, string) {
	if strings.HasPrefix(str, prefix) {
		res := strings.Trim(str, "\n")
		res = strings.Replace(res, prefix, "", -1)

		return true, res
	}

	return false, ""
}

func isClientHandshake(msg string) (bool, string) {
	return parseByPrefix(msg, prefixHsClient)
}

func isServerHandshake(msg string) (bool, string) {
	return parseByPrefix(msg, prefixHsServer)
}

func isOnlineStatusRequest(msg string) (bool, string) {
	return parseByPrefix(msg, prefixOnlineStatusReq)
}

func isOnlineStatusResponce(msg string) (bool, string) {
	return parseByPrefix(msg, prefixOnlineStatusResp)
}
