# Broker Architecture Documentation

This document provides a comprehensive overview of the MQTT broker's internal architecture, covering message flow, concurrency patterns, data structures, and component interactions.

## Table of Contents

1. [High-Level Architecture](#high-level-architecture)
2. [Transport Layer](#transport-layer)
3. [Connection Lifecycle](#connection-lifecycle)
4. [Client Structure](#client-structure)
5. [Packet Processing Pipeline](#packet-processing-pipeline)
6. [Message Routing](#message-routing)
7. [Subscription Trie](#subscription-trie)
8. [Session Management](#session-management)
9. [QoS and Inflight Tracking](#qos-and-inflight-tracking)
10. [Retained Messages](#retained-messages)
11. [Hook System](#hook-system)
12. [Concurrency Patterns](#concurrency-patterns)

---

## High-Level Architecture

```mermaid
block
  columns 1
  block:broker["Broker Core"]
    columns 2
    clients["clients map"]
    sessions["SessionManager"]
    subs["SubscriptionTree"]
    retained["retained map"]
    hooks["Hooks Manager"]:2
  end
  space
  block:transport["Transport Layer"]
    columns 3
    tcp["TCP"]
    tls["TLS"]
    ws["WebSocket"]
  end
  space
  net["Network Layer (net.Conn)"]

  net --> transport
  transport --> broker
```

The broker follows a transport-agnostic design where any `net.Conn` implementation can be used. The core broker receives connections from listeners and handles them uniformly regardless of the underlying transport.

---

## Transport Layer

### Listener Types

| Transport | File | Description |
|-----------|------|-------------|
| TCP | `listener.go:20` | Plain TCP listener using `net.Listen` |
| TLS | `listener.go:40` | TLS-encrypted TCP using `tls.Listen` |
| WebSocket | `websocket.go:26` | WebSocket over HTTP/HTTPS with `gorilla/websocket` |

### WebSocket Adapter

WebSocket connections are wrapped in a `wsConn` adapter (`websocket.go:138`) that implements `net.Conn`:

```mermaid
block
  columns 1
  block:wsconn["wsConn implements net.Conn"]
    columns 2
    embedded["*websocket.Conn (embedded)"]
    reader["reader io.Reader"]
    remoteAddr["remoteAddr string"]
    mu["mu sync.Mutex"]
  end
```

Methods: `Read()` and `Write()` handle binary WebSocket messages transparently.

---

## Connection Lifecycle

```mermaid
flowchart TD
    A[Accept conn] --> B[HandleConnection<br/>new goroutine + panic recovery]
    B --> C{MaxConnections<br/>check}
    C -->|Limit reached| X[Close connection]
    C -->|OK| D[newClient<br/>Create Client]
    D --> E[Set connect timeout]
    E --> F[Read CONNECT packet]
    F --> G[handleConnect<br/>Auth hooks / Session mgmt / Send CONNACK]
    G --> H[Clear read deadline]
    H --> I[Start loops]
    I --> J[writeLoop<br/>goroutine]
    I --> K[readLoop<br/>blocking]
    K --> L[handlePacket<br/>Dispatch to handlers]
    J --> M[handleDisconnect]
    L --> M
    M --> N[Remove from clients map]
    N --> O[Publish will message if any]
    O --> P[Handle session clean/persist]
    P --> Q[Clean up subscriptions]
```

---

## Client Structure

Each connected client is represented by a `Client` struct (`client.go:16`):

| Category | Field | Description |
|----------|-------|-------------|
| **Network** | `conn` | `net.Conn` - underlying connection |
| **Network** | `reader` | `*packet.Reader` - buffered decoder |
| **Identity** | `clientID` | Client identifier from CONNECT |
| **Identity** | `username` | Username from CONNECT |
| **Identity** | `version` | MQTT protocol version |
| **Identity** | `cleanStart` | Clean session flag |
| **Identity** | `keepAlive` | Keep-alive interval |
| **Identity** | `properties` | MQTT 5.0 properties |
| **State** | `connected` | `atomic.Bool` - connection state |
| **State** | `closed` | `atomic.Bool` - prevent double-close |
| **Outbound** | `outbound` | `chan Packet` (cap: 256, non-blocking) |
| **Inflight** | `inFlightOut` | `map[uint16]*inflightMsg` - QoS 1/2 out |
| **Inflight** | `inFlightIn` | `map[uint16]*Publish` - QoS 2 in |
| **Inflight** | `nextPacketID` | `uint16` - packet ID counter |
| **Will** | `will` | `*Publish` - will message |
| **Will** | `willProps` | `*Properties` - will properties |
| **Will** | `willDelay` | `uint32` - will delay interval |
| **Keep-Alive** | `lastActivity` | `time.Time` - for timeout detection |
| **References** | `session` | `*Session` - persistent state |
| **References** | `broker` | `*Broker` - parent broker |
| **References** | `ctx, cancel` | Context for cancellation |

**Mutexes:** `packetIDMu` protects inflight maps and nextPacketID; `keepAliveMu` protects lastActivity

### Client Goroutines

Each client spawns two goroutines after successful CONNECT. Both have panic recovery with stack trace logging to prevent a single client from crashing the broker:

```mermaid
flowchart LR
    subgraph ReadLoop["readLoop() - blocking"]
        R1["reader.ReadPacket()"]
        R2["broker.handlePacket()"]
        R3["On error/EOF:<br/>handleDisconnect()"]
        R1 --> R2 --> R1
        R2 -.-> R3
    end

    subgraph WriteLoop["writeLoop() - goroutine"]
        W0["defer recover() // panic safe"]
        W1["for pkt := range outbound"]
        W2["encode(pkt)"]
        W3["conn.Write(buf)"]
        W0 --> W1 --> W2 --> W3 --> W1
    end

    ReadLoop -->|"outbound channel"| WriteLoop
```

---

## Packet Processing Pipeline

```mermaid
flowchart TD
    A["reader.ReadPacket()"] --> B["broker.handlePacket()<br/>Type switch dispatch"]
    B --> C[PUBLISH<br/>publish.go:10]
    B --> D[SUBSCRIBE<br/>subscribe.go:8]
    B --> E[PUBACK<br/>client.ackInflight]
    B --> F[PUBREC<br/>client.Send PUBREL]
    B --> G[PUBREL<br/>releaseInbound route]
    B --> H[PINGREQ<br/>Send PINGRESP]
```

### PUBLISH Processing Flow

```mermaid
flowchart TD
    A[handlePublish] --> B[Validate topic<br/>no wildcards]
    B --> C[hooks.OnPublish<br/>Authorization check]
    C --> D[hooks.OnPublishReceived<br/>Can modify packet]
    D --> E{QoS Level}
    E -->|QoS 0| F[No ack needed]
    E -->|QoS 1| G[Send PUBACK]
    E -->|QoS 2| H[trackInbound<br/>Send PUBREC<br/>wait for PUBREL]
    F --> I[Handle retain / Route message]
    G --> I
    H -.->|on PUBREL| I
```

---

## Message Routing

The `routeMessage()` function (`publish.go:127`) handles message delivery to all matching subscribers:

```mermaid
flowchart TD
    A["routeMessage(sender, pkt)"] --> B["subscriptions.Match(topic)"]
    B --> C["For each subscriber"]
    C --> D{NoLocal check}
    D -->|Skip if sender == subscriber| C
    D -->|OK| E["hooks.CanRead(subscriber, topic)"]
    E -->|Denied| C
    E -->|OK| F["Determine delivery QoS<br/>min(message QoS, subscription QoS)"]
    F --> G["Create delivery packet<br/>Copy MQTT 5.0 properties<br/>Add subscription ID"]
    G --> H["hooks.OnPublishDeliver()<br/>Can modify or reject"]
    H --> I["Assign packet ID<br/>for QoS > 0"]
    I --> J{Client connected?}
    J -->|Yes| K["client.Send()<br/>non-blocking"]
    J -->|No & QoS > 0| L["session.QueueMessage()"]
    K --> M{Queue full & QoS > 0?}
    M -->|Yes| L
    M -->|No| C
    L --> C
```

---

## Subscription Trie

The subscription tree (`subscriptions.go`) is a trie-based data structure optimized for topic matching with wildcards:

```
Example subscriptions:
  - ClientA: "home/+/temperature"
  - ClientB: "home/kitchen/#"
  - ClientC: "$share/group1/home/kitchen/temperature"

                              ┌─────────┐
                              │  root   │
                              │ trieNode│
                              └────┬────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                              ▼
              ┌─────────┐                    ┌─────────┐
              │ "home"  │                    │ "$share"│
              │ trieNode│                    │ trieNode│
              └────┬────┘                    └────┬────┘
                   │                              │
          ┌────────┴────────┐                     ▼
          ▼                 ▼               ┌─────────┐
    ┌─────────┐       ┌─────────┐           │"group1" │
    │   "+"   │       │"kitchen"│           │ trieNode│
    │ trieNode│       │ trieNode│           └────┬────┘
    └────┬────┘       └────┬────┘                │
         │                 │                     ▼
         ▼                 ▼               ┌─────────┐
   ┌───────────┐     ┌─────────┐           │ "home"  │
   │"temperat-"│     │   "#"   │           │ trieNode│
   │   "ure"   │     │ trieNode│           └────┬────┘
   │ trieNode  │     │         │                │
   │           │     │subscrib-│                ▼
   │subscribers│     │ers:     │           ... (continues)
   │: ClientA  │     │ ClientB │
   └───────────┘     └─────────┘
```

### trieNode Structure

```mermaid
block
  columns 1
  block:trie["trieNode"]
    columns 1
    children["children map[string]*trieNode"]
    subscribers["subscribers map[*Client]*Subscriber"]
    shared["shared map[string]*sharedGroup"]
  end
```

### Shared Subscription Group

```mermaid
block
  columns 1
  block:group["sharedGroup"]
    columns 1
    subs["subscribers []*Subscriber"]
    nextIdx["nextIdx int (round-robin)"]
    mu["mu sync.Mutex"]
  end
```

The `next()` method atomically selects `subscribers[nextIdx % len]` and increments the counter.

### Topic Matching Algorithm

```mermaid
flowchart TD
    A["Match('home/kitchen/temperature')"] --> B["matchRecursive(root, levels, idx=0)"]
    B --> C["At each level:"]
    C --> D["1. Try exact match → children['home']"]
    C --> E["2. If not $SYS topic at idx=0:"]
    E --> F["Try '+' wildcard → continue"]
    E --> G["Try '#' wildcard → ADD ALL & return"]
    D --> H["At final level:"]
    H --> I["Add all node.subscribers"]
    H --> J["For shared groups: pick one via round-robin"]
    H --> K["Check for trailing '#'"]
```

---

## Session Management

Sessions persist subscription state and queued messages across client reconnections:

```mermaid
block
  columns 1
  block:session["Session (mu sync.RWMutex)"]
    columns 2
    clientID["ClientID string"]
    subscriptions["subscriptions map"]
    pending["pending []*pendingMsg"]
    client["client *Client"]
    createdAt["createdAt int64"]
    expiresAt["expiresAt int64"]
  end
```

> `QueueMessage()` enforces `Config.MaxSessionQueue` limit (default: 1000)

### SessionManager

| Method | Description |
|--------|-------------|
| `Get(clientID)` | Returns existing session or nil |
| `GetOrCreate(clientID)` | Returns existing or creates new |
| `Create(clientID)` | Creates new, replacing any existing |
| `Delete(clientID)` | Removes session |
| `CleanExpired()` | Removes expired sessions |

### Session Lifecycle

```mermaid
flowchart TD
    subgraph Connect["CONNECT received"]
        A{CleanStart?}
        A -->|"=1"| B["sessions.Create()<br/>New session"]
        A -->|"=0"| C["sessions.GetOrCreate()"]
        C --> D{Session exists?}
        D -->|Yes| E["Resume:<br/>Restore subs<br/>Drain pending"]
        D -->|No| F["Create new session"]
    end

    subgraph Disconnect["DISCONNECT/Error"]
        G{CleanStart?}
        G -->|"=1"| H["UnsubscribeAll()<br/>sessions.Delete()<br/>hooks.OnSessionEnded()"]
        G -->|"=0"| I["session.SetClient(nil)<br/>Keep session for<br/>future reconnect"]
    end
```

---

## QoS and Inflight Tracking

### QoS 0 (At most once)

```mermaid
sequenceDiagram
    participant P as Publisher
    participant B as Broker
    participant S as Subscriber
    P->>B: PUBLISH (QoS 0)
    Note over B: No ack
    B->>S: PUBLISH (QoS 0)
```

### QoS 1 (At least once)

```mermaid
sequenceDiagram
    participant P as Publisher
    participant B as Broker
    participant S as Subscriber
    P->>B: PUBLISH (QoS 1)
    B->>P: PUBACK
    Note over B: routeMessage() → trackInflight(pkt)
    B->>S: PUBLISH (QoS 1)
    S->>B: PUBACK
    Note over B: ackInflight(id) → delete from inFlightOut
```

### QoS 2 (Exactly once)

```mermaid
sequenceDiagram
    participant P as Publisher
    participant B as Broker
    P->>B: PUBLISH (QoS 2)
    Note over B: trackInbound(pkt) → DO NOT ROUTE YET
    B->>P: PUBREC
    P->>B: PUBREL
    Note over B: releaseInbound(id) → routeMessage()
    B->>P: PUBCOMP
```

### Inflight Data Structures

```mermaid
block
  columns 2
  block:out["inFlightOut map[uint16]*inflightMsg"]
    columns 1
    pkt["pkt *Publish"]
    timestamp["timestamp time.Time"]
    retries["retries int"]
  end
  block:in["inFlightIn map[uint16]*Publish"]
    columns 1
    inVal["Stores QoS 2 packets awaiting PUBREL"]
  end
```

> `trackInflight()` enforces `Config.MaxInflight` limit (default: 65535). Returns false if limit reached; message queued to session instead.

---

## Retained Messages

```mermaid
flowchart TD
    subgraph Storage["Storage (retain.go:11)"]
        A["PUBLISH with Retain=1"] --> B["storeRetained(topicName, pkt)"]
        B --> C["1. Try hooks.StoreRetained()<br/>(external storage: Redis, DB)"]
        C --> D["2. If no hook → in-memory:"]
        D --> E{"len(payload) == 0?"}
        E -->|Yes| F["delete(retained, topicName)"]
        E -->|No| G["retained[topicName] = copy(pkt)"]
    end

    subgraph Retrieval["Retrieval (retain.go:37)"]
        H["SUBSCRIBE to filter"] --> I["sendRetained(client, filter, maxQoS)"]
        I --> J["1. Try hooks.GetRetained()"]
        J --> K["2. If no hook → scan in-memory:"]
        K --> L["for topicName, msg := range retained"]
        L --> M{"topic.Match(filter, topicName)?"}
        M -->|Yes| N["deliverRetained(client, msg, maxQoS)"]
        M -->|No| L
    end
```

---

## Hook System

The hook system provides extensibility points throughout the message lifecycle:

```mermaid
block
  columns 1
  block:hooks["Hooks Manager (mu sync.RWMutex)"]
    columns 3
    all["all []Hook"]
    onConnect["onConnect"]
    onConnected["onConnected"]
    onDisconnect["onDisconnect"]
    onSubscribe["onSubscribe"]
    onPublish["onPublish"]
    onPublishReceived["onPublishReceived"]
    onPublishDeliver["onPublishDeliver"]
    canRead["canRead"]
    storeRetained["storeRetained"]
    getRetained["getRetained"]
    onSessionCreated["onSessionCreated"]
    onSessionEnded["onSessionEnded"]
    space
  end
```

> Cached lists by event type enable fast O(1) dispatch to relevant hooks

### Hook Interface

| Method | Category | Description |
|--------|----------|-------------|
| `ID() string` | Lifecycle | Hook identifier |
| `Init(opts, config) error` | Lifecycle | Initialize hook |
| `Stop() error` | Lifecycle | Cleanup hook |
| `Provides(event byte) bool` | Lifecycle | Which events this hook handles |
| `OnConnect(ctx, client, pkt) error` | Connection | Auth, can reject |
| `OnConnected(ctx, client)` | Connection | Notification |
| `OnDisconnect(ctx, client, err)` | Connection | Notification |
| `OnSubscribe(ctx, client, subs) (subs, err)` | Authorization | Filter/reject subs |
| `OnPublish(ctx, client, pkt) error` | Authorization | Authorize publish |
| `CanRead(ctx, client, topic) bool` | Authorization | Per-message authz |
| `OnPublishReceived(ctx, client, pkt) (*pkt, err)` | Transform | Transform/drop inbound |
| `OnPublishDeliver(ctx, sub, pkt) (*pkt, err)` | Transform | Transform/drop outbound |
| `OnSessionCreated(ctx, client)` | Session | New session created |
| `OnSessionResumed(ctx, client)` | Session | Existing session resumed |
| `OnSessionEnded(ctx, clientID)` | Session | Session deleted |
| `StoreRetained(ctx, topic, pkt) error` | Retained | External storage |
| `GetRetained(ctx, filter) ([]*pkt, error)` | Retained | External retrieval |

### Hook Dispatch Patterns

| Pattern | Hooks | Behavior |
|---------|-------|----------|
| **Fail-Fast** | OnConnect, OnPublish | First error stops chain, rejects operation |
| **Transform Chain** | OnPublishReceived, OnPublishDeliver, OnSubscribe | Each hook can modify; error rejects |
| **Notify All** | OnConnected, OnDisconnect, OnSessionCreated | Fire-and-forget, no error handling |
| **First Provider** | StoreRetained, GetRetained | First hook to handle wins |
| **All Must Pass** | CanRead | Any hook can deny |

---

## Concurrency Patterns

### Mutexes (sync.RWMutex)

| Location | Mutex | Protected Data |
|----------|-------|----------------|
| `broker.go` | `clientsMu` | `clients map[string]*Client` |
| `broker.go` | `retainedMu` | `retained map[string]*Publish` |
| `session.go` | `Session.mu` | subscriptions, pending, client |
| `session.go` | `SessionMgr.mu` | sessions map |
| `subscriptions.go` | `SubTree.mu` | Entire trie structure |
| `subscriptions.go` | `sharedGroup.mu` | Round-robin index only |
| `client.go` | `packetIDMu` | nextPacketID, inFlightOut/In |
| `client.go` | `keepAliveMu` | lastActivity timestamp |
| `hooks.go` | `Hooks.mu` | All hook slices |
| `websocket.go` | `wsConn.mu` | WebSocket reader state |

### Channels

| Location | Channel | Purpose |
|----------|---------|---------|
| `client.go:37` | `outbound chan Packet` | Packet queue to writeLoop (capacity: 256, non-blocking send) |
| `listener.go:17` | `closed chan struct{}` | Signal listener shutdown |
| `websocket.go:21` | `closed chan struct{}` | Signal WebSocket listener shutdown |

### Atomics

| Location | Atomic | Purpose |
|----------|--------|---------|
| `client.go:33` | `connected Bool` | Connection state flag |
| `client.go:34` | `closed Bool` | Prevent double-close |

### Sync Primitives

| Location | Type | Purpose |
|----------|------|---------|
| `broker.go:80` | `sync.WaitGroup` | Track client goroutines |
| `listener.go:16` | `sync.WaitGroup` | Wait for accept loop exit |
| `websocket.go:20` | `sync.WaitGroup` | Wait for HTTP server exit |
| `reader.go:195` | `sync.Pool` | Buffer pool for encoding |

### Lock Granularity Design

```mermaid
flowchart TD
    A["Broker Level<br/>clientsMu, retainedMu"] --> B["SessionManager<br/>mu (sessions map only)"]
    A --> C["SubscriptionTree<br/>mu (trie structure)"]
    B --> D["Session<br/>mu (per-session data)<br/>independent locking"]
```

**Benefits:**
- Clients can operate independently
- Publishing doesn't block connecting
- Session access is per-client, not global
- Subscription tree has single lock but short critical sections

### Non-Blocking Send Pattern

```go
func (c *Client) Send(pkt packet.Packet) bool {
    if c.closed.Load() {          // Fast path: check atomic flag
        return false
    }

    select {
    case c.outbound <- pkt:       // Try to send
        return true
    default:                      // Channel full
        return false              // Drop (QoS 0) or queue to session
    }
}
```

This pattern ensures:
- Publishing never blocks on slow clients
- Memory is bounded (channel capacity = 256)
- QoS > 0 messages can be queued to session for retry

---

## Complete Message Flow Example

```mermaid
sequenceDiagram
    participant P as Publisher
    participant B as Broker
    participant S as Subscriber

    P->>B: PUBLISH (QoS 1)
    Note over B: readLoop()<br/>handlePacket()<br/>handlePublish()
    Note over B: topic.ValidateName() ✓<br/>hooks.OnPublish() ✓<br/>hooks.OnPublishReceived()
    B->>P: PUBACK

    Note over B: routeMessage()<br/>subscriptions.Match("topic")

    Note over B: For each subscriber:<br/>NoLocal check<br/>hooks.CanRead()<br/>min(pubQoS, subQoS)<br/>hooks.OnPublishDeliver()<br/>subscriber.nextID()<br/>subscriber.Send(pkt)<br/>trackInflight(pkt)

    B->>S: PUBLISH (QoS 1)
    Note over S: readLoop()<br/>handlePacket()
    S->>B: PUBACK
    Note over B: handlePuback()<br/>ackInflight(id)
```

---

## File Reference

| File | Primary Responsibility |
|------|------------------------|
| `broker.go` | Core broker struct, lifecycle, connection dispatch |
| `client.go` | Client struct, read/write loops, packet ID management |
| `listener.go` | TCP/TLS listeners, Server convenience wrapper |
| `websocket.go` | WebSocket listener and net.Conn adapter |
| `connect.go` | CONNECT packet handling, session setup |
| `handlers.go` | Packet type dispatcher, PINGREQ, disconnect handling |
| `publish.go` | PUBLISH handling, QoS flows, message routing |
| `subscribe.go` | SUBSCRIBE/UNSUBSCRIBE handling |
| `subscriptions.go` | Trie-based subscription tree, wildcard matching |
| `session.go` | Session struct, SessionManager, pending messages |
| `retain.go` | Retained message storage and retrieval |
| `hook.go` | Hook interface definition, HookBase, ClientInfo |
| `hooks.go` | Hooks manager, event dispatch |
| `pkg/topic/topic.go` | Topic validation, wildcard matching, shared subs |
| `pkg/packet/reader.go` | Buffered packet reader, buffer pool |
