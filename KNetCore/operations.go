package KNetCore

import (
	"context"

	"github.com/KrigMoon/KJournal"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

//region Сеть (Соединение)

//region^ Подключение

func (knc *KNetCore) Connect(remoteAddr string, node *knetpb.Node, password string) error {
	conn, err := grpc.NewClient(remoteAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	client := knetpb.NewNetNodeClient(conn)
	var remoteNode *knetpb.Node
	if node == nil {
		ans, err := client.Register(context.Background(), &knetpb.RegisterRequest{MyNode: knc.MyNode, Password: password})
		if err != nil {
			knc.journal.Write(KJournal.NewRecord(
				"KNetCore",
				knetpb.RecordLevel_ERROR.String(),
				knetpb.RecordCode_NET.String(),
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

	knc.journal.Write(KJournal.NewRecord(
		"KNetCore",
		knetpb.RecordLevel_INFO.String(),
		knetpb.RecordCode_NET.String(),
		"Подключён к узлу "+remoteNode.Id,
	))

	return nil
}

//endregion^

//region^ Отключение

func (knc *KNetCore) RemovePeer(id string) {
	pA, ok := knc.peers.Load(id)
	if !ok {
		return
	}

	p := pA.(*Peer)

	//* Прощание с той стороной
	if p.Client != nil {
		if _, err := p.Client.Disconnect(context.Background(), knc.MyNode); err != nil {
			knc.journal.Write(KJournal.NewRecord(
				"KNetCore",
				knetpb.RecordLevel_WARNING.String(),
				knetpb.RecordCode_NET.String(),
				"Disconnect "+id+": "+err.Error(),
			))
		}
	}

	//* Уборка у себя
	if p.Conn != nil {
		p.Conn.Close()
	}
	knc.peers.Delete(id)

	knc.journal.Write(KJournal.NewRecord(
		"KNetCore",
		knetpb.RecordLevel_INFO.String(),
		knetpb.RecordCode_NET.String(),
		"Узел "+id+" отключён",
	))
}

//endregion^

//endregion

//region Сеть (Пиры)

func (knc *KNetCore) findPeer(to string) *Peer {
	if pA, ok := knc.peers.Load(to); ok {
		return pA.(*Peer)
	}

	if knc.MyNode.Type != knetpb.TypeNode_CLIENT {
		return nil
	}

	var found *Peer
	knc.peers.Range(func(_, v any) bool {
		p := v.(*Peer)
		if p.Node.Type != knetpb.TypeNode_SERVER {
			return true
		}
		ans, err := p.Client.GetPeers(knc.ctx, knc.MyNode)
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

func (knc *KNetCore) GetPeerInfoList() []PeerInfo {
	var result []PeerInfo

	knc.peers.Range(func(_, v any) bool {
		p := v.(*Peer)
		info := PeerInfo{Node: p.Node}

		// если пир — сервер, спросить у него его пиров
		if p.Node.Type == knetpb.TypeNode_SERVER {
			ans, err := p.Client.GetPeers(knc.ctx, knc.MyNode)
			if err != nil {
				knc.journal.Write(KJournal.NewRecord(
					"KNetCore",
					knetpb.RecordLevel_WARNING.String(),
					knetpb.RecordCode_NET.String(),
					"GetPeers у "+p.Node.Id+": "+err.Error(),
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

//endregion
