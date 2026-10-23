package KNet

import (
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
	"context"
	"sync"

	"github.com/KrigMoon/KCommandParser"
	"github.com/KrigMoon/KJournal"
	"github.com/admpub/once"
)

type KNetSettings struct {
	Journal             *KJournal.KJournal
	ExternalCommandsMap map[string]struct{}
}

type KNet struct {
	// Журнал и настройки
	journal  *KJournal.KJournal
	settings KNetSettings
	// Жизненый цикл
	status     knetpb.Status
	ctx        context.Context
	cancel     context.CancelFunc
	stopOnce   once.Once
	stopAwait  chan struct{}
	proccesses sync.WaitGroup
	// Конвееры
	commands         chan *KCommandParser.Command
	externalCommands chan *KCommandParser.Command
	responses        chan *KCommandParser.Response
	// Карты
	localCommandsMap    map[string]CommandContainer
	externalCommandsMap map[string]struct{}
}

func (kn *KNet) Run(settings KNetSettings) {
	if kn.status != knetpb.Status_OFF {
		return
	}

	//Установка настроек
	kn.settings = settings

	defer func() {
		go kn.stop()
	}()

	//* Инициализация
	kn.ctx, kn.cancel = context.WithCancel(context.Background())
	kn.stopOnce.Reset()
	kn.stopAwait = make(chan struct{})
	kn.journal = kn.settings.Journal
	kn.journal.AddWriter()

	//* Инициализация конвееров
	kn.commands = make(chan *KCommandParser.Command, 256)
	kn.externalCommands = make(chan *KCommandParser.Command, 256)
	kn.responses = make(chan *KCommandParser.Response, 256)

	//* Инициализация карт
	kn.localCommandsMap = map[string]CommandContainer{}
	kn.externalCommandsMap = kn.settings.ExternalCommandsMap

	//* Включение
	kn.status = knetpb.Status_RUNNING
	kn.journal.Write(KJournal.NewRecord("KNet", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNet RUNNING"))

	go kn.executer()

	kn.status = knetpb.Status_ON
	kn.journal.Write(KJournal.NewRecord("KNet", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNet ON"))
}

func (kn *KNet) EndWork() {
	if kn.status != knetpb.Status_ON {
		return
	}
	kn.stopOnce.Do(func() {
		kn.cancel()
		<-kn.stopAwait
	})
}

func (kn *KNet) stop() {
	<-kn.ctx.Done()

	//* Включение
	kn.status = knetpb.Status_STOP
	kn.journal.Write(KJournal.NewRecord("KNet", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNet STOP"))
	kn.proccesses.Wait()
	kn.status = knetpb.Status_OFF
	kn.journal.Write(KJournal.NewRecord("KNet", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNet OFF"))

	kn.journal.DelWriter()
	close(kn.stopAwait)
}
