package KNet

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/KrigMoon/KNet/KNetCore"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"

	"github.com/KrigMoon/KCommandParser"
	"github.com/KrigMoon/KJournal"
	"github.com/admpub/once"
)

type KNetSettings struct {
	Journal             *KJournal.KJournal
	ExternalCommandsMap map[string]struct{}
	AutoRun             []string
	MyNode              *knetpb.Node
	Password            string
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
	packages         chan *knetpb.Package
	// Карты
	localCommandsMap    map[string]CommandContainer
	externalCommandsMap map[string]struct{}
	// Сетевое ядро
	knc KNetCore.KNetCore
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
	kn.packages = make(chan *knetpb.Package, 256)

	//* Инициализация карт
	kn.localCommandsMap = map[string]CommandContainer{
		"/connect": {
			Check: func(cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
				if len(cmd.Args) == 2 {
					return nil, true
				} else {
					return &KCommandParser.Response{
						CommandID:       cmd.ID,
						Level:           knetpb.RecordLevel_ERROR.String(),
						Time:            time.Now(),
						ResponseMessage: "Неверное кол-во аргументов",
					}, true
				}

			},
			Execute: kn.cmdConnect,
		},
		"/disconnect": {
			Check: func(cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
				if len(cmd.Args) == 2 {
					return nil, true
				} else {
					return &KCommandParser.Response{
						CommandID:       cmd.ID,
						Level:           knetpb.RecordLevel_ERROR.String(),
						Time:            time.Now(),
						ResponseMessage: "Неверное кол-во аргументов",
					}, true
				}

			},
			Execute: kn.cmdDisconnect,
		},
		"/send": {
			Check: func(cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
				if len(cmd.Args) >= 2 {
					return nil, true
				} else {
					return &KCommandParser.Response{
						CommandID:       cmd.ID,
						Level:           knetpb.RecordLevel_ERROR.String(),
						Time:            time.Now(),
						ResponseMessage: "Неверное кол-во аргументов",
					}, true
				}

			},
			Execute: func(cc CMDCore, cmd *KCommandParser.Command) *KCommandParser.Response {
				return kn.cmdSend(cc, cmd, knetpb.PackageType_MESSAGE)
			},
		},
		"/json": {
			Check: func(cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
				// Минимум 2 аргумента: команда + JSON
				if len(cmd.Args) < 2 {
					return &KCommandParser.Response{
						CommandID:       cmd.ID,
						Level:           knetpb.RecordLevel_ERROR.String(),
						Time:            time.Now(),
						ResponseMessage: "Неверное кол-во аргументов",
					}, true
				}
				// Проверяем, что второй аргумент - валидный JSON
				if !json.Valid([]byte(cmd.Args[1].Value())) {
					return &KCommandParser.Response{
						CommandID:       cmd.ID,
						Level:           knetpb.RecordLevel_ERROR.String(),
						Time:            time.Now(),
						ResponseMessage: "Невалидный JSON",
					}, true
				}
				return nil, true
			},
			Execute: func(cc CMDCore, cmd *KCommandParser.Command) *KCommandParser.Response {
				return kn.cmdSend(cc, cmd, knetpb.PackageType_JSON)
			},
		},
		"/remote": {
			Check: func(cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
				if len(cmd.Args) < 2 {
					return &KCommandParser.Response{
						CommandID:       cmd.ID,
						Level:           knetpb.RecordLevel_ERROR.String(),
						Time:            time.Now(),
						ResponseMessage: "Неверное кол-во аргументов",
					}, true
				}
				return nil, true
			},
			Execute: func(cc CMDCore, cmd *KCommandParser.Command) *KCommandParser.Response {
				return kn.cmdSend(cc, cmd, knetpb.PackageType_COMMAND)
			},
		},
	}
	kn.externalCommandsMap = kn.settings.ExternalCommandsMap

	//* Включение
	kn.status = knetpb.Status_RUNNING
	kn.journal.Write(KJournal.NewRecord("KNet", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNet RUNNING"))

	go kn.executer()
	go kn.readerPKG()

	kn.status = knetpb.Status_ON
	kn.journal.Write(KJournal.NewRecord("KNet", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNet ON"))

	kn.knc.Run(KNetCore.KNetCoreSettings{
		Journal:  kn.journal,
		MyNode:   kn.settings.MyNode,
		Password: kn.settings.Password,
	})

	if kn.settings.AutoRun != nil {
		for _, cmd := range kn.settings.AutoRun {
			kn.Execute(cmd)
		}
	}
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

	kn.knc.EndWork()

	//* Включение
	kn.status = knetpb.Status_STOP
	kn.journal.Write(KJournal.NewRecord("KNet", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNet STOP"))
	kn.proccesses.Wait()
	kn.status = knetpb.Status_OFF
	kn.journal.Write(KJournal.NewRecord("KNet", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNet OFF"))

	kn.journal.DelWriter()
	close(kn.stopAwait)
}
