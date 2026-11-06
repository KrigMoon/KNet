package KNetCore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/KrigMoon/KNet/Proto"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

//region Network (Connection)

//region^ Connection

func (knc *KNetCore) Connect(remoteAddr string, node *knetpb.Node, password string) error {
	conn, err := grpc.NewClient(remoteAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	client := knetpb.NewNetNodeClient(conn)
	var remoteNode *knetpb.Node
	if node == nil {
		if password == "" {
			return fmt.Errorf("Connect: Password is required")
		}
		sum := sha256.Sum256([]byte(password))
		password = hex.EncodeToString(sum[:])

		ans, err := client.Register(context.Background(), &knetpb.RegisterRequest{MyNode: knc.settings.MyNode, Password: password})
		if err != nil {
			knc.settings.Journal.Write(Proto.NewRecord(
				"KNetCore",
				knetpb.RecordLevel_ERROR,
				knetpb.RecordCode_NET,
				err.Error(),
			))
			conn.Close()
			return err
		}
		remoteNode = ans.MyNode
	} else {
		remoteNode = node
	}

	knc.peers.Store(remoteNode.Id, &Peer{Node: remoteNode, Client: client, Conn: conn})

	knc.settings.Journal.Write(Proto.NewRecord(
		"KNetCore",
		knetpb.RecordLevel_INFO,
		knetpb.RecordCode_NET,
		"Connected to the node "+remoteNode.Id,
	))

	return nil
}

//endregion^

//region^ Disconnection

func (knc *KNetCore) RemovePeer(id string) {
	pA, ok := knc.peers.Load(id)
	if !ok {
		return
	}

	p := pA.(*Peer)

	//* Farewell to the other side
	if p.Client != nil {
		if _, err := p.Client.Disconnect(context.Background(), knc.settings.MyNode); err != nil {
			knc.settings.Journal.Write(Proto.NewRecord(
				"KNetCore",
				knetpb.RecordLevel_WARNING,
				knetpb.RecordCode_NET,
				"Disconnect "+id+": "+err.Error(),
			))
		}
	}

	//* Cleaning at home
	if p.Conn != nil {
		p.Conn.Close()
	}
	knc.peers.Delete(id)

	knc.settings.Journal.Write(Proto.NewRecord(
		"KNetCore",
		knetpb.RecordLevel_INFO,
		knetpb.RecordCode_NET,
		"Node "+id+" disabled",
	))
}

//endregion^

//endregion

//region Network (Peers)

func (knc *KNetCore) findPeer(to string) *Peer {
	if pA, ok := knc.peers.Load(to); ok {
		return pA.(*Peer)
	}

	if knc.settings.MyNode.Type != knetpb.TypeNode_CLIENT {
		return nil
	}

	var found *Peer
	knc.peers.Range(func(_, v any) bool {
		p := v.(*Peer)
		if p.Node.Type != knetpb.TypeNode_SERVER {
			return true
		}
		ans, err := p.Client.GetPeers(knc.ctx, knc.settings.MyNode)
		if err != nil {
			return true
		}
		for _, peer := range ans.Peers {
			if peer.Id == to {
				found = p
				return false
			}
		}
		return true
	})
	return found
}

func (knc *KNetCore) GetPeerList() []PeerInfo {
	var result []PeerInfo

	knc.peers.Range(func(_, v any) bool {
		p := v.(*Peer)
		info := PeerInfo{Node: p.Node}

		//* if the peer is a server, ask him for his peers
		if p.Node.Type == knetpb.TypeNode_SERVER {
			ans, err := p.Client.GetPeers(knc.ctx, knc.settings.MyNode)
			if err != nil {
				knc.settings.Journal.Write(Proto.NewRecord(
					"KNetCore",
					knetpb.RecordLevel_WARNING,
					knetpb.RecordCode_NET,
					"GetPeers at "+p.Node.Id+": "+err.Error(),
				))
			} else {
				for _, peer := range ans.Peers {
					info.PeerList = append(info.PeerList, peer)
				}
			}
		}

		result = append(result, info)
		return true
	})

	return result
}

func (knc *KNetCore) IsPeerKnown(id string) bool {
	return knc.findPeer(id) != nil
}

//endregion
