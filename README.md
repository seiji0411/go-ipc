<div align="center">

# 🔌 go-ipc

**Bidirectional inter-process communication in Go, built for streaming blockchain events to trading bots.**

![Go](https://img.shields.io/badge/Go-1.19+-00ADD8?logo=go&logoColor=white)
![Transport](https://img.shields.io/badge/transport-named%20pipes%20%7C%20unix%20sockets-6E40C9)
![Built on](https://img.shields.io/badge/built%20on-golang--ipc-informational)

</div>

---

## 📖 Overview

`go-ipc` connects a single **node-side IPC server** to any number of **bot clients** running as separate processes on the same machine. Each bot gets two dedicated channels:

| Channel | Direction | Purpose |
|---|---|---|
| **Subscribe** | Server ➜ Bot | Broadcast blocks, logs and pending transactions |
| **Submit** | Bot ➜ Server | Send signed raw transactions back to the node |

All transport is handled by [`james-barrow/golang-ipc`](https://github.com/james-barrow/golang-ipc), which uses **named pipes on Windows** and **Unix domain sockets on Linux/macOS**, so there is no TCP/HTTP overhead between the node and its bots.

---

## 🏗️ Architecture

```mermaid
flowchart LR
    subgraph NODE["🖥️ Node process (server.go)"]
        direction TB
        MAIN[["bot_main_ipc<br/><i>registration pipe</i>"]]
        REG["addBotClient()"]
        BC["BroadcastLog()"]
        SUBMIT_IN["submitTransaction()"]
        HC["checkClientStatus()<br/><i>every 5s</i>"]
        MAP[("botClients map<br/>RWMutex")]
        MAIN --> REG --> MAP
        BC --> MAP
        HC --> MAP
    end

    subgraph BOT1["🤖 Bot process A (client.go)"]
        direction TB
        SA[["bot&lt;ts&gt;_&lt;rand&gt;<br/><i>subscribe pipe</i>"]]
        CHA["chan IPCMessage"]
        SA --> CHA
    end

    subgraph BOT2["🤖 Bot process B (client.go)"]
        direction TB
        SB[["bot&lt;ts&gt;_&lt;rand&gt;<br/><i>subscribe pipe</i>"]]
        CHB["chan IPCMessage"]
        SB --> CHB
    end

    MAP -- "events" --> SA
    MAP -- "events" --> SB
    SA -. "ping / 3s" .-> MAP
    SB -. "ping / 3s" .-> MAP
    BOT1 -- "raw tx via &lt;name&gt;_submit" --> SUBMIT_IN
    BOT2 -- "raw tx via &lt;name&gt;_submit" --> SUBMIT_IN
```

### Pipes per bot

Every connected bot results in **three** pipes over its lifetime:

```mermaid
flowchart LR
    subgraph S["Node (IPC Server)"]
        S1["ipc.Server<br/>bot_main_ipc"]
        S2["ipc.Client<br/>(Subscribe)"]
        S3["ipc.Server<br/>&lt;name&gt;_submit"]
    end
    subgraph C["Bot (IPC Client)"]
        C1["ipc.Client<br/>(server)"]
        C2["ipc.Server<br/>&lt;name&gt;"]
        C3["ipc.Client<br/>(Submit)"]
    end
    C1 == "① register (closed after handshake)" ==> S1
    S2 == "② subscribe: events + CreateSubmit" ==> C2
    C3 == "③ submit: raw transactions" ==> S3
```

> 💡 Notice the role reversal: for the **subscribe** pipe the *bot* hosts the server and the *node* dials in. This lets the node push to each bot over its own dedicated pipe.

---

## 🔄 Communication Workflow

### 1. Handshake & registration

```mermaid
sequenceDiagram
    autonumber
    participant Bot as 🤖 Bot (nodeipcclient)
    participant Main as 📮 bot_main_ipc
    participant Node as 🖥️ Node (nodeipcserver)

    Note over Node: Run() starts server on "bot_main_ipc"
    Bot->>Main: ipc.StartClient("bot_main_ipc")
    Note over Bot: wait 2s
    Bot->>Bot: StartServer("bot<ts>_<rand>")  ← subscribe pipe
    Note over Bot: wait 2s
    Bot->>Main: MsgTypeNewClient (1) · data = "bot<ts>_<rand>"
    Main->>Node: addBotClient(name)
    Node->>Bot: ipc.StartClient(name)  ← dials bot's subscribe pipe
    Node->>Node: StartServer(name + "_submit")
    Node->>Node: botClients[name] = {Submit, Subscribe}
    Note over Node: wait 2s
    Node->>Bot: MsgTypeCreateSubmit (4) · data = "<name>_submit"
    Bot->>Node: ipc.StartClient("<name>_submit")  ← submit pipe ready
    Note over Bot: 2s after registering, close bot_main_ipc connection
```

### 2. Event streaming (Server ➜ Bot)

```mermaid
sequenceDiagram
    participant Src as ⛓️ Event source
    participant Node as 🖥️ Node
    participant A as 🤖 Bot A
    participant B as 🤖 Bot B
    participant App as 📊 Bot logic

    Src->>Node: new block / log / pending tx
    Node->>Node: BroadcastLog(data) · RLock
    par fan-out (one goroutine per bot)
        Node->>A: MsgTypeMessage (2) · JSON IPCMessage
    and
        Node->>B: MsgTypeMessage (2) · JSON IPCMessage
    end
    A->>A: json.Unmarshal → IPCMessage
    A->>App: channel <- IPCMessage
    App->>App: switch MessageType<br/>BlockStart · Log · BlockEnd · PendingTx
```

### 3. Transaction submission (Bot ➜ Server)

```mermaid
sequenceDiagram
    participant App as 📊 Bot logic
    participant Bot as 🤖 Bot
    participant Node as 🖥️ Node

    App->>App: build & sign tx (go-ethereum)
    App->>Bot: SubmitTxn(signedTx.MarshalBinary())
    Bot->>Node: MsgTypeMessage (2) on "<name>_submit"
    Node->>Node: submitTransaction(name, rawTx)
    Note right of Node: TODO: forward to the node's tx pool
```

### 4. Heartbeat & eviction

```mermaid
stateDiagram-v2
    [*] --> Registered: MsgTypeNewClient
    Registered --> Alive: first ping
    Alive --> Alive: MsgTypePing every 3s<br/>(updates LastPingTs)
    Alive --> Stale: no ping for > 10s
    Registered --> Stale: no ping for > 10s
    Stale --> [*]: checkClientStatus() (every 5s)<br/>Close Submit + Subscribe,<br/>delete from botClients
```

---

## 📨 Message Protocol

Every frame on the wire is a `golang-ipc` message with an integer `MsgType` and a `[]byte` payload.

### Transport-level message types

| Value | Constant | Direction | Payload |
|:---:|---|---|---|
| `0` | `MsgTypeNone` | — | ignored |
| `1` | `MsgTypeNewClient` | Bot ➜ Node (main pipe) | bot's subscribe pipe name |
| `2` | `MsgTypeMessage` | both | JSON `IPCMessage` (subscribe) or raw tx bytes (submit) |
| `3` | `MsgTypePing` | Bot ➜ Node (subscribe pipe) | `[]byte{1}` |
| `4` | `MsgTypeCreateSubmit` | Node ➜ Bot (subscribe pipe) | submit pipe name |

### Application-level envelope

`MsgTypeMessage` frames on the subscribe pipe carry a JSON envelope whose inner `messageType` tells the bot how to decode `messageData`:

```go
type IPCMessage struct {
    MessageType MsgType `json:"messageType"`
    MessageData []byte  `json:"messageData"` // base64 in JSON
}
```

| Value | Constant | `messageData` decodes to |
|:---:|---|---|
| `5` | `MsgTypeBlockStart` | `IPCBlock{BlockNumber, BlockTimestamp}` |
| `6` | `MsgTypeLog` | go-ethereum `types.Log` |
| `7` | `MsgTypeBlockEnd` | — (marker) |
| `8` | `MsgTypePendingTx` | `PendingTx{TxHash, From, To, Nonce, Gas*, Data, Logs, …}` |

```mermaid
flowchart TD
    F["ipc.Message"] --> T{"MsgType?"}
    T -- "2 Message" --> J["json.Unmarshal → IPCMessage"]
    T -- "3 Ping" --> P["update LastPingTs"]
    T -- "4 CreateSubmit" --> CS["dial &lt;name&gt;_submit"]
    J --> M{"MessageType?"}
    M -- "5" --> B1["IPCBlock — block start"]
    M -- "6" --> B2["types.Log"]
    M -- "7" --> B3["block end"]
    M -- "8" --> B4["PendingTx + simulated logs"]
```

---

## 📁 Project Structure

```text
go-ipc/
├── server.go               # Demo node: runs IPC server, broadcasts every 5s
├── client.go               # Demo bot: subscribes and prints events, can submit txs
├── wsClient.go             # Standalone WebSocket client for comparison (no IPC)
├── nodeipcserver/          # Node-side library
│   ├── server.go           #   Shared(), Run(), BroadcastLog(), health checks
│   ├── constant/           #   BotMainIPC = "bot_main_ipc"
│   ├── message/            #   Client/Server wrappers + message types
│   └── utils/
└── nodeipcclient/          # Bot-side library
    ├── client.go           #   Shared(), Run(chan), SubmitTxn()
    ├── constant/
    ├── message/            #   Client/Server wrappers + IPCMessage, PendingTx
    └── utils/
```

---

## 🚀 Getting Started

### Prerequisites

- Go **1.19+**

```bash
go mod download
```

### Run the server (node side)

```bash
go run server.go
```

### Run one or more clients (bot side)

In separate terminals:

```bash
go run client.go
```

> ⚠️ `server.go`, `client.go` and `wsClient.go` each declare their own `main`, so run them **file by file** as shown above rather than `go run .` / `go build ./...`.

### Expected output

```text
# server
2026-10-06T10:00:00Z IpcServer Start
2026-10-06T10:00:04Z IpcServer received new bot client client bot1759744804000_42137
2026-10-06T10:00:06Z IpcServer sending create submit to client client bot1759744804000_42137 submit bot1759744804000_42137_submit
2026-10-06T10:00:10Z IpcServer BroadcastLog start clients 1 len 12

# client
Client starting
2026-10-06T10:00:04Z Client sending add new bot: bot1759744804000_42137
2026-10-06T10:00:04Z Client sent add new bot
```

> ℹ️ The demo server broadcasts the plain string `"Hello Client"`. Since the bot expects a JSON `IPCMessage`, it will log `JSON parse Error` for those frames. Real producers should send a JSON-encoded `IPCMessage` (see below).

---

## 🧩 Usage as a Library

### Node side — broadcast events

```go
import (
    "encoding/json"
    nodeipc "go-ipc/nodeipcserver"
    "go-ipc/nodeipcserver/message"
)

func main() {
    go nodeipc.Shared().Run()

    block, _ := json.Marshal(message.IPCBlock{BlockNumber: 123, BlockTimestamp: 1700000000})
    envelope, _ := json.Marshal(message.IPCMessage{
        MessageType: message.MsgTypeBlockStart,
        MessageData: block,
    })
    nodeipc.Shared().BroadcastLog(envelope)
}
```

### Bot side — consume events & submit transactions

```go
import (
    nodeipc "go-ipc/nodeipcclient"
    "go-ipc/nodeipcclient/message"
)

func main() {
    events := make(chan message.IPCMessage)
    if err := nodeipc.Shared().Run(events); err != nil {
        panic(err)
    }

    for ev := range events {
        switch ev.MessageType {
        case message.MsgTypeBlockStart: /* ... */
        case message.MsgTypeLog:        /* ... */
        case message.MsgTypePendingTx:  /* ... */
        }
    }

    // anywhere, once the submit pipe exists:
    // nodeipc.Shared().SubmitTxn(rawSignedTx)
}
```

---

## ⏱️ Timing Reference

| Constant | Value | Where |
|---|---|---|
| Handshake settle delays | 2s each | `nodeipcclient.Run`, `nodeipcserver.addBotClient` |
| Bot ping interval | 3s | `nodeipcclient.startClientSchedule` |
| Health-check interval | 5s | `nodeipcserver.startServerSchedule` |
| Eviction timeout | 10s without ping | `nodeipcserver.checkClientStatus` |
| Demo broadcast interval | 5s | `server.go` |

---

## 🙏 Acknowledgements

- [james-barrow/golang-ipc](https://github.com/james-barrow/golang-ipc) — cross-platform IPC transport
- [ethereum/go-ethereum](https://github.com/ethereum/go-ethereum) — transaction and log types
