package KNet

import (
	"time"

	"github.com/KrigMoon/KCommandParser"
	"github.com/KrigMoon/KJournal"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

type CMDCore interface {
	Connect(remoteAddr string, node *knetpb.Node, password string) error
	RemovePeer(id string)
	Write(pkg *knetpb.Package)
	GetMyNode() *knetpb.Node
}

func (kn *KNet) cmdConnect(cc CMDCore, cmd *KCommandParser.Command) *KCommandParser.Response {
	addr := cmd.Args[0].Value()
	password := cmd.Args[1].Value()

	if err := cc.Connect(addr, nil, password); err != nil {
		kn.journal.Write(KJournal.NewRecord(
			"KNet",
			knetpb.RecordLevel_ERROR.String(),
			knetpb.RecordCode_CommandCenter.String(),
			"Сбой подключения: "+err.Error(),
		))
	}

	return &KCommandParser.Response{
		CommandID:       cmd.ID,
		Level:           knetpb.RecordLevel_INFO.String(),
		Time:            time.Now(),
		ResponseMessage: "Успешное подключение",
	}
}

func (kn *KNet) cmdDisconnect(cc CMDCore, cmd *KCommandParser.Command) *KCommandParser.Response {
	id := cmd.Args[0].Value()
	cc.RemovePeer(id)

	return &KCommandParser.Response{
		CommandID:       cmd.ID,
		Level:           knetpb.RecordLevel_INFO.String(),
		Time:            time.Now(),
		ResponseMessage: "Успешное отключение",
	}
}

func (kn *KNet) cmdSend(cc CMDCore, cmd *KCommandParser.Command, pkgType knetpb.PackageType) *KCommandParser.Response {
	to := cmd.Args[0].Value()
	data := cmd.Args[1].Value()

	cc.Write(&knetpb.Package{
		From: cc.GetMyNode().Id,
		To:   to,
		Type: pkgType,
		Data: data,
	})

	return &KCommandParser.Response{
		CommandID:       cmd.ID,
		Level:           knetpb.RecordLevel_INFO.String(),
		Time:            time.Now(),
		ResponseMessage: cmd.Name + ": пакет отправлен узлу " + to,
	}
}
