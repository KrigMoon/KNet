package KNetCore

import (
	"context"
	"sync"

	"github.com/KrigMoon/KJournal"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
	"github.com/admpub/once"
)

type KNetCoreSettings struct {
	Journal *KJournal.KJournal
}

type KNetCore struct {
	// Журнал и настройки
	journal  *KJournal.KJournal
	settings KNetCoreSettings
	// Жизненый цикл
	status     knetpb.Status
	ctx        context.Context
	cancel     context.CancelFunc
	stopOnce   once.Once
	stopAwait  chan struct{}
	proccesses sync.WaitGroup
}

func (knc *KNetCore) Run(settings KNetCoreSettings) {
	if knc.status != knetpb.Status_OFF {
		return
	}

	//Установка настроек
	knc.settings = settings

	defer func() {
		go knc.stop()
	}()

	//* Инициализация
	knc.ctx, knc.cancel = context.WithCancel(context.Background())
	knc.stopOnce.Reset()
	knc.stopAwait = make(chan struct{})
	knc.journal = knc.settings.Journal
	knc.journal.AddWriter()

	//* Включение
	knc.status = knetpb.Status_RUNNING
	knc.journal.Write(KJournal.NewRecord("KNetCore", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNetCore RUNNING"))

	knc.status = knetpb.Status_ON
	knc.journal.Write(KJournal.NewRecord("KNetCore", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNetCore ON"))

}

func (knc *KNetCore) EndWork() {
	if knc.status != knetpb.Status_ON {
		return
	}
	knc.stopOnce.Do(func() {
		knc.cancel()
		<-knc.stopAwait
	})
}

func (knc *KNetCore) stop() {
	<-knc.ctx.Done()

	//* Включение
	knc.status = knetpb.Status_STOP
	knc.journal.Write(KJournal.NewRecord("KNetCore", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNetCore STOP"))
	knc.proccesses.Wait()
	knc.status = knetpb.Status_OFF
	knc.journal.Write(KJournal.NewRecord("KNetCore", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNetCore OFF"))

	knc.journal.DelWriter()
	close(knc.stopAwait)
}
