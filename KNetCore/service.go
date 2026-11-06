package KNetCore

import (
	"context"
	"net"

	"github.com/KrigMoon/KJournal"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

//region Metrics

type KNetCoreMetrics struct {
	Status     knetpb.Status
	Entry      struct{ Len, Cap int }
	Exit       struct{ Len, Cap int }
	PeersCount int
}

func (knc *KNetCore) GetMetrics() KNetCoreMetrics {
	m := KNetCoreMetrics{
		Status: knc.status,
		Entry: struct {
			Len int
			Cap int
		}{Len: len(knc.entry), Cap: cap(knc.entry)},
		Exit: struct {
			Len int
			Cap int
		}{Len: len(knc.exit), Cap: cap(knc.exit)},
	}

	knc.peers.Range(func(_, v any) bool {
		m.PeersCount++
		p := v.(*Peer)
		if p.Node.Type == knetpb.TypeNode_SERVER && p.Client != nil {
			if ans, err := p.Client.GetPeers(context.Background(), knc.settings.MyNode); err == nil {
				m.PeersCount += len(ans.Peers)
			}
		}
		return true
	})

	return m
}

//endregion

//region Utility Methods and Getters

//region^ Utility Methods

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "0.0.0.0"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return "0.0.0.0"
}

//endregion^

//region^ Getters

func (knc *KNetCore) GetJournal() *KJournal.KJournal {
	return knc.settings.Journal
}

func (knc *KNetCore) GetMyNode() *knetpb.Node {
	return knc.settings.MyNode
}

//endregion^

//endregion
