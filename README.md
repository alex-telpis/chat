# Chat
Server-Client app on Go

### Installation:
```sh
$ go get github.com/alex-telpis/chat
```

### Usage:

Initiate your first server with empty parameters and run it on desired port
```go
app := chat.NewServer("", "")
app.Run("4441")
```

For cluster mode: add another server instance:
- initiate with own addr as first param, it will be used to introduce the instance to cluster
- pass address of any already running instance as 2nd param. This server will "guide" just created instance through the cluster by sharing addresses of other members
- run instance on port that was mentioned in ownAddr
...
- add as many instances as needed
```go
ownAddr := "127.0.0.1:4442"
guideAddr := "127.0.0.1:4441"
app := chat.NewServer(ownAddr, guideAddr)
app.Run("4442")
```

Prepare client by passing user name. Run it on server address
```go
app := chat.NewClient("JohnDoe")
app.Run("127.0.0.1:4442")
```

### Lanucher Examples:
Server
```go
package main

import (
	"fmt"
	"os"

	"github.com/alex-telpis/chat"
)

func main() {
	args := os.Args

	if len(args) < 1 {
		fmt.Println("Usage: chat-server <port> <opt:own addr> <opt:guide addr>")
		os.Exit(0)
	}

	if len(args) >= 3 {
		// cluster mode
		app := chat.NewServer(args[2], args[3])
		app.Run(args[1])
	} else {
		app := chat.NewServer("", "")
		app.Run(args[1])
	}
}
```

Client
```go
package main

import (
	"fmt"
	"os"

	"github.com/alex-telpis/chat"
)

func main() {
	args := os.Args

	if len(args) < 3 {
		fmt.Println("Usage: chat-client <serverAddr> <username>")
		os.Exit(0)
	}

	if len(args[1]) < 3 {
		fmt.Println("Username shoud be at least 3 chars")
		os.Exit(0)
	}

	app := chat.NewClient(args[2])
	app.Run(args[1])
}

```
