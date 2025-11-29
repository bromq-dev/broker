package gossip

import (
	"encoding/json"

	"github.com/bromq-dev/broker/pkg/cluster/types"
	"github.com/hashicorp/memberlist"
)

// gossipDelegate implements memberlist.Delegate for state synchronization.
type gossipDelegate struct {
	store *Store
}

func (d *gossipDelegate) NodeMeta(limit int) []byte {
	meta, _ := json.Marshal(map[string]string{
		"addr": d.store.getRoutingAddr(),
	})
	return meta
}

func (d *gossipDelegate) NotifyMsg(data []byte) {
	var msg gossipMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	d.store.handleGossipMsg(msg)
}

func (d *gossipDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	return d.store.broadcasts.GetBroadcasts(overhead, limit)
}

func (d *gossipDelegate) LocalState(join bool) []byte {
	d.store.subsMu.RLock()
	d.store.retainedMu.RLock()
	d.store.nodesMu.RLock()
	defer d.store.subsMu.RUnlock()
	defer d.store.retainedMu.RUnlock()
	defer d.store.nodesMu.RUnlock()

	state := struct {
		Subs     map[string]map[string]*nodeSubInfo `json:"s"`
		Retained map[string]*retainedMsg            `json:"r"`
		Nodes    map[string]*types.NodeInfo         `json:"n"`
	}{
		Subs:     d.store.subs,
		Retained: d.store.retained,
		Nodes:    d.store.nodes,
	}

	data, _ := json.Marshal(state)
	return data
}

func (d *gossipDelegate) MergeRemoteState(buf []byte, join bool) {
	var state struct {
		Subs     map[string]map[string]*nodeSubInfo `json:"s"`
		Retained map[string]*retainedMsg            `json:"r"`
		Nodes    map[string]*types.NodeInfo         `json:"n"`
	}
	if err := json.Unmarshal(buf, &state); err != nil {
		return
	}

	// Merge subscriptions
	d.store.subsMu.Lock()
	for filter, nodes := range state.Subs {
		if d.store.subs[filter] == nil {
			d.store.subs[filter] = make(map[string]*nodeSubInfo)
		}
		for nodeID, info := range nodes {
			d.store.subs[filter][nodeID] = info
		}
	}
	d.store.subsMu.Unlock()

	// Merge retained
	d.store.retainedMu.Lock()
	for topic, msg := range state.Retained {
		d.store.retained[topic] = msg
	}
	d.store.retainedMu.Unlock()

	// Merge nodes
	d.store.nodesMu.Lock()
	for nodeID, info := range state.Nodes {
		d.store.nodes[nodeID] = info
	}
	d.store.nodesMu.Unlock()
}

// gossipEvents implements memberlist.EventDelegate for node join/leave events.
type gossipEvents struct {
	store *Store
}

func (e *gossipEvents) NotifyJoin(node *memberlist.Node) {
	e.store.log.Info("node joined", "node", node.Name)

	// Add to nodes map
	var meta struct {
		Addr string `json:"addr"`
	}
	json.Unmarshal(node.Meta, &meta)

	e.store.nodesMu.Lock()
	e.store.nodes[node.Name] = &types.NodeInfo{
		ID:   node.Name,
		Addr: meta.Addr,
	}
	e.store.nodesMu.Unlock()
}

func (e *gossipEvents) NotifyLeave(node *memberlist.Node) {
	e.store.log.Info("node left", "node", node.Name)

	// Remove subscriptions for departed node
	e.store.subsMu.Lock()
	for filter, nodes := range e.store.subs {
		delete(nodes, node.Name)
		if len(nodes) == 0 {
			delete(e.store.subs, filter)
		}
	}
	e.store.subsMu.Unlock()

	// Remove from nodes map
	e.store.nodesMu.Lock()
	delete(e.store.nodes, node.Name)
	e.store.nodesMu.Unlock()
}

func (e *gossipEvents) NotifyUpdate(node *memberlist.Node) {}

// gossipBroadcast implements memberlist.Broadcast.
type gossipBroadcast struct {
	data []byte
}

func (b *gossipBroadcast) Invalidates(other memberlist.Broadcast) bool {
	return false
}

func (b *gossipBroadcast) Message() []byte {
	return b.data
}

func (b *gossipBroadcast) Finished() {}
