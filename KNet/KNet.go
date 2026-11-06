package KNet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KrigMoon/KCommandParser"
	"github.com/KrigMoon/KJournal"
	"github.com/KrigMoon/KNet/KNet/Commands"
	"github.com/KrigMoon/KNet/KNetCore"
	"github.com/KrigMoon/KNet/Proto"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
	"github.com/admpub/once"
)

//region Structure, settings and constructor

//region^ KNet's Settings

type KNSettings struct {
	Journal             *KJournal.KJournal
	Core                KNetCore.KNCSettings
	ExternalCommandsMap map[string]struct{}
	AutoRun             []string
	FileStorageDir      string
	RemoteControl       bool
}

// desc: Validates settings
func (kns *KNSettings) validate() error {
	if kns.Journal == nil {
		return fmt.Errorf("KNSettings: Journal is required")
	}
	return nil
}

// desc: Normalizing data and setting default values
func (kns *KNSettings) normalize() {
	if kns.ExternalCommandsMap == nil {
		kns.ExternalCommandsMap = make(map[string]struct{})
	}

	//* FileStorageDir: relative paths are counted from the binary's directory,
	//* then everything is reduced to an absolute path.
	base := ""
	if exe, err := os.Executable(); err == nil {
		base = filepath.Dir(exe)
	}

	dir := kns.FileStorageDir
	if dir == "" {
		dir = "./"
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(base, dir)
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	kns.FileStorageDir = dir
}

//endregion^

//region^ KNet

type KNet struct {
	//* Settings and status
	settings KNSettings
	status   knetpb.Status
	//* Lifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	awaitStop chan struct{}
	stopOnce  once.Once
	processes sync.WaitGroup
	//* Net Core
	knc KNetCore.KNetCore
	//* Commands Cursor
	cursor      atomic.Value
	cursorQueue chan *Commands.CursorProgram
	//* Maps
	localCommandsMap map[string]Commands.CommandContainer
	//* Pipelines
	responses chan *KCommandParser.Response
	readers   sync.Map
	//* Readers packages
	readersWait  sync.WaitGroup
	readersCount atomic.Int32
}

func NewKNet() KNet {
	return KNet{}
}

//endregion^

//endregion

//region Lifecycle

func (kn *KNet) Run(settings KNSettings) error {
	if kn.status != knetpb.Status_OFF {
		return fmt.Errorf("KNet is already running")
	}

	//* Initialization and check settings
	kn.settings = settings
	if err := kn.settings.validate(); err != nil {
		return err
	}
	kn.settings.normalize()

	kn.settings.Journal.NewWriter()

	kn.settings.Journal.Write(Proto.NewRecord("KNet", knetpb.RecordLevel_INFO, knetpb.RecordCode_LIFECIRCLE, "KNet RUNNING"))
	kn.status = knetpb.Status_RUNNING

	//* Initialization lifecycle
	kn.ctx, kn.cancel = context.WithCancel(context.Background())
	kn.awaitStop = make(chan struct{})
	kn.stopOnce.Reset()

	//* Initialization pipelines, cursor and map
	kn.cursor.Store(Commands.CursorNil)
	kn.cursorQueue = make(chan *Commands.CursorProgram, 512)
	kn.responses = make(chan *KCommandParser.Response, 256)
	kn.localCommandsMap = Commands.GetCommandsMap(Commands.CommandsSettings{
		FileStorageDir: kn.settings.FileStorageDir,
	})

	//* Launch of the shutdown mechanism
	go kn.stop()

	go kn.executerPrograms()

	kn.settings.Journal.Write(Proto.NewRecord("KNet", knetpb.RecordLevel_INFO, knetpb.RecordCode_LIFECIRCLE, "KNet ON"))
	kn.status = knetpb.Status_ON

	kn.knc = KNetCore.NewKNetCore()
	if err := kn.knc.Run(kn.settings.Core); err != nil {
		return err
	}

	go kn.routePackages()

	if err := kn.CreateFileCollector(); err != nil {
		kn.settings.Journal.Write(Proto.NewRecord(
			"KNet",
			knetpb.RecordLevel_ERROR,
			knetpb.RecordCode_CommandCenter,
			"FileReader error: "+err.Error()))
	}

	time.Sleep(1000 * time.Millisecond)

	if kn.settings.AutoRun != nil {
		for _, cmd := range kn.settings.AutoRun {
			kn.Execute(cmd)
			time.Sleep(200 * time.Millisecond)
		}
	}

	return nil
}

func (kn *KNet) Endwork() error {
	if kn.status != knetpb.Status_ON {
		return fmt.Errorf("KNet is already stopped")
	}

	kn.stopOnce.Do(func() {
		kn.cancel()
		<-kn.awaitStop
	})

	return nil
}

func (kn *KNet) stop() {
	<-kn.ctx.Done()

	kn.knc.Endwork()

	kn.settings.Journal.Write(Proto.NewRecord("KNet", knetpb.RecordLevel_INFO, knetpb.RecordCode_LIFECIRCLE, "KNet STOP"))
	kn.status = knetpb.Status_STOP

	kn.processes.Wait()

	kn.settings.Journal.Write(Proto.NewRecord("KNet", knetpb.RecordLevel_INFO, knetpb.RecordCode_LIFECIRCLE, "KNet OFF"))
	kn.status = knetpb.Status_OFF

	kn.settings.Journal.DelWriter()

	close(kn.awaitStop)
}

//endregion
