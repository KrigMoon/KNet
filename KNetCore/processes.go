package KNetCore

import (
	"context"

	"github.com/KrigMoon/KJournal"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

func (knc *KNetCore) writer() {
	knc.processes.Add(1)
	defer knc.processes.Done()

	for {
		select {
		case <-knc.ctx.Done():
			return
		case pkg := <-knc.entryPackages:
			p := knc.findPeer(pkg.To)
			if p == nil {
				knc.journal.Write(KJournal.NewRecord(
					"KNetCore",
					knetpb.RecordLevel_ERROR.String(),
					knetpb.RecordCode_NET.String(),
					"Получатель "+pkg.To+" не найден"),
				)
				continue
			}
			if ack, err := p.Client.Send(context.Background(), pkg); err != nil {
				knc.journal.Write(KJournal.NewRecord(
					"KNetCore",
					knetpb.RecordLevel_ERROR.String(),
					knetpb.RecordCode_NET.String(), "Ошибка отправки узлу "+pkg.To+": "+err.Error()),
				)
			} else if !ack.Ok {
				knc.journal.Write(KJournal.NewRecord(
					"KNetCore",
					knetpb.RecordLevel_ERROR.String(),
					knetpb.RecordCode_NET.String(),
					"Ошибка отправки узлу "+pkg.To+": "+ack.Error),
				)
			}
		}
	}
}
