package KNetCore

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/KrigMoon/KJournal"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

//region Сеть (Соединение)

//region^ Подключение

func (knc *KNetCore) Register(ctx context.Context, req *knetpb.RegisterRequest) (*knetpb.RegisterResponse, error) {
	if knc.MyNode.Type == req.MyNode.Type {
		return &knetpb.RegisterResponse{
			MyNode:  nil,
			Success: false,
		}, fmt.Errorf("Подключение возможно только для разных типов узлов")
	}
	if knc.settings.Password != req.Password {
		return &knetpb.RegisterResponse{
			MyNode:  nil,
			Success: false,
		}, fmt.Errorf("Неверный пароль")
	}
	knc.journal.Write(KJournal.NewRecord(
		"KNetCore",
		knetpb.RecordLevel_INFO.String(),
		knetpb.RecordCode_NET.String(),
		fmt.Sprintf("Принято новое подключение от узла %s", req.MyNode.Id),
	))

	remote := req.MyNode
	remoteAddr := net.JoinHostPort(remote.ListeningIP, strconv.Itoa(int(remote.ListeningPort)))

	knc.Connect(remoteAddr, remote, "")

	return &knetpb.RegisterResponse{
		MyNode:  knc.MyNode,
		Success: true,
	}, nil
}

//endregion^

//region^ Отключение

func (knc *KNetCore) Disconnect(ctx context.Context, node *knetpb.Node) (*knetpb.Void, error) {
	if _, ok := knc.peers.Load(node.Id); !ok {
		return &knetpb.Void{}, nil // уже нет — не ошибка
	}

	// убираем у себя, но БЕЗ ответного Disconnect, чтобы не было рекурсии
	pA, _ := knc.peers.Load(node.Id)
	p := pA.(*Peer)
	if p.Conn != nil {
		p.Conn.Close()
	}
	knc.peers.Delete(node.Id)

	knc.journal.Write(KJournal.NewRecord(
		"KNetCore",
		knetpb.RecordLevel_INFO.String(),
		knetpb.RecordCode_NET.String(),
		"Узел "+node.Id+" отключился",
	))

	return &knetpb.Void{}, nil
}

//endregion^

//endregion

//region Сеть (Связь)

func (knc *KNetCore) Send(ctx context.Context, pkg *knetpb.Package) (*knetpb.SendResponse, error) {
	if knc.MyNode.Id != pkg.To {
		// пакет не мне — могу переслать, если я SERVER
		if knc.MyNode.Type == knetpb.TypeNode_SERVER {
			knc.Write(pkg)
			return &knetpb.SendResponse{Ok: true}, nil
		} else {
			// пакет не мне, я CLIENT — отказ
			knc.journal.Write(KJournal.NewRecord(
				"KNetCore",
				knetpb.RecordLevel_ERROR.String(),
				knetpb.RecordCode_NET.String(),
				"Узел "+knc.MyNode.Id+" не может маршрутизировать пакет для "+pkg.To,
			))
			return &knetpb.SendResponse{Ok: false, Error: "узел не маршрутизирует"}, nil
		}
	}

	select {
	case knc.exitPackages <- pkg:
	default:
		knc.journal.Write(KJournal.NewRecord(
			"KNetCore",
			knetpb.RecordLevel_WARNING.String(),
			knetpb.RecordCode_DATA.String(),
			"Внимание! Буфер приёма пакетов полон.",
		))
		return &knetpb.SendResponse{Ok: false, Error: "Буфер приёма пакетов полон. Пожалуйста подождите"}, nil
	}
	return &knetpb.SendResponse{Ok: true}, nil
}

//endregion

//region Сеть (Пиры)

func (knc *KNetCore) GetPeers(ctx context.Context, req *knetpb.Node) (*knetpb.PeersResponse, error) {
	if knc.MyNode.Type != knetpb.TypeNode_SERVER {
		return nil, fmt.Errorf("GetPeers доступен только на сервере")
	}

	peers := make([]*knetpb.Node, 0)
	knc.peers.Range(func(_, v any) bool {
		p := v.(*Peer)
		if p.Node.Id == req.Id {
			return true // исключаем спрашивающего
		}
		peers = append(peers, p.Node)
		return true
	})

	return &knetpb.PeersResponse{Peers: peers}, nil
}

//endregion
