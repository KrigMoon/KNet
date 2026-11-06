package KNetCore

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/KrigMoon/KNet/Proto"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

//region Network (Connection)

//region^ Connection

func (knc *KNetCore) Register(ctx context.Context, req *knetpb.RegisterRequest) (*knetpb.RegisterResponse, error) {
	if knc.settings.MyNode.Type == req.MyNode.Type {
		return &knetpb.RegisterResponse{
			MyNode:  nil,
			Success: false,
		}, fmt.Errorf("Connection is only possible for different types of nodes")
	}
	if knc.settings.Password != req.Password {
		return &knetpb.RegisterResponse{
			MyNode:  nil,
			Success: false,
		}, fmt.Errorf("Invalid password")
	}
	knc.settings.Journal.Write(Proto.NewRecord(
		"KNetCore",
		knetpb.RecordLevel_INFO,
		knetpb.RecordCode_NET,
		fmt.Sprintf("A new connection has been accepted from the node %s", req.MyNode.Id),
	))

	remote := req.MyNode
	remoteAddr := net.JoinHostPort(remote.ListeningIP, strconv.Itoa(int(remote.ListeningPort)))

	knc.Connect(remoteAddr, remote, "")

	return &knetpb.RegisterResponse{
		MyNode:  knc.settings.MyNode,
		Success: true,
	}, nil
}

//endregion^

//region^ Disconnection

func (knc *KNetCore) Disconnect(ctx context.Context, node *knetpb.Node) (*knetpb.Void, error) {
	if _, ok := knc.peers.Load(node.Id); !ok {
		return &knetpb.Void{}, nil
	}

	//* we clean it up at home, but WITHOUT the Disconnect response, so that there is no recursion
	pA, _ := knc.peers.Load(node.Id)
	p := pA.(*Peer)
	if p.Conn != nil {
		p.Conn.Close()
	}
	knc.peers.Delete(node.Id)

	knc.settings.Journal.Write(Proto.NewRecord(
		"KNetCore",
		knetpb.RecordLevel_INFO,
		knetpb.RecordCode_NET,
		"Node "+node.Id+" disconnected",
	))

	return &knetpb.Void{}, nil
}

//endregion^

//endregion

//region Network (Communication)

func (knc *KNetCore) Send(ctx context.Context, pkg *knetpb.Package) (*knetpb.SendResponse, error) {
	if knc.settings.MyNode.Id != pkg.To {
		//* the package is not for me — I can forward it if I am a SERVER
		if knc.settings.MyNode.Type == knetpb.TypeNode_SERVER {
			knc.Write(pkg)
			return &knetpb.SendResponse{Ok: true}, nil
		} else {
			//* the package is not for me, I am a CLIENT — rejection
			knc.settings.Journal.Write(Proto.NewRecord(
				"KNetCore",
				knetpb.RecordLevel_ERROR,
				knetpb.RecordCode_NET,
				"Node "+knc.settings.MyNode.Id+" cannot route a package to "+pkg.To,
			))
			return &knetpb.SendResponse{Ok: false, Error: "the node does not route"}, nil
		}
	}

	select {
	case knc.exit <- pkg:
	default:
		knc.settings.Journal.Write(Proto.NewRecord(
			"KNetCore",
			knetpb.RecordLevel_WARNING,
			knetpb.RecordCode_DATA,
			"Attention! The packet receiving buffer is full.",
		))
		return &knetpb.SendResponse{Ok: false, Error: "The packet receiving buffer is full. Please wait"}, nil
	}
	return &knetpb.SendResponse{Ok: true}, nil
}

//endregion

//region Network (Peers)

func (knc *KNetCore) GetPeers(ctx context.Context, req *knetpb.Node) (*knetpb.PeersResponse, error) {
	if knc.settings.MyNode.Type != knetpb.TypeNode_SERVER {
		return nil, fmt.Errorf("GetPeers available only on the server")
	}

	peers := make([]*knetpb.Node, 0)
	knc.peers.Range(func(_, v any) bool {
		p := v.(*Peer)
		if p.Node.Id == req.Id {
			return true
		}
		peers = append(peers, p.Node)
		return true
	})

	return &knetpb.PeersResponse{Peers: peers}, nil
}

//endregion
