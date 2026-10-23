package KNetCore

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/KrigMoon/KJournal"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
	"github.com/admpub/once"
	"google.golang.org/grpc"
)

type Peer struct {
	Node   *knetpb.Node
	Client knetpb.NetNodeClient
	Conn   *grpc.ClientConn
}

type PeerInfo struct {
	Node     *knetpb.Node
	PeerList []*knetpb.Node
}

type KNetCoreSettings struct {
	Journal  *KJournal.KJournal
	MyNode   *knetpb.Node
	Password string
}

type KNetCore struct {
	knetpb.UnimplementedNetNodeServer
	// Журнал и настройки
	journal  *KJournal.KJournal
	settings KNetCoreSettings
	// Жизненый цикл
	status    knetpb.Status
	ctx       context.Context
	cancel    context.CancelFunc
	stopOnce  once.Once
	stopAwait chan struct{}
	processes sync.WaitGroup
	// Конвееры
	entryPackages chan *knetpb.Package // send
	exitPackages  chan *knetpb.Package // read
	//* Сетевой узел
	MyNode *knetpb.Node
	//* gRPC
	srv *grpc.Server
	//* База узлов
	peers sync.Map
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
	knc.MyNode = knc.settings.MyNode

	//* Включение
	knc.status = knetpb.Status_RUNNING
	knc.journal.Write(KJournal.NewRecord("KNetCore", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNetCore RUNNING"))

	knc.runServer()

	knc.status = knetpb.Status_ON
	knc.journal.Write(KJournal.NewRecord("KNetCore", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNetCore ON"))

	go knc.writer()
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

	knc.processes.Wait()

	knc.stopServer()

	knc.status = knetpb.Status_OFF
	knc.journal.Write(KJournal.NewRecord("KNetCore", knetpb.RecordLevel_INFO.String(), knetpb.RecordCode_LIFECIRCLE.String(), "KNetCore OFF"))

	knc.journal.DelWriter()
	close(knc.stopAwait)
}

//region Хелперы запуска

func (knc *KNetCore) runServer() {
	if knc.MyNode.ListeningIP == "" {
		knc.MyNode.ListeningIP = localIP()
	}

	addr := net.JoinHostPort(knc.MyNode.ListeningIP, strconv.Itoa(int(knc.MyNode.ListeningPort)))

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		knc.journal.Write(KJournal.NewRecord(
			"KNetCore",
			knetpb.RecordLevel_ERROR.String(),
			knetpb.RecordCode_NET.String(),
			fmt.Sprintf("listen: %v", err),
		))
	}

	if knc.MyNode.ListeningPort == 0 {
		if tcpAddr, ok := lis.Addr().(*net.TCPAddr); ok {
			knc.MyNode.ListeningPort = int32(tcpAddr.Port)
		}
	}

	knc.srv = grpc.NewServer()
	knetpb.RegisterNetNodeServer(knc.srv, knc)

	knc.journal.Write(KJournal.NewRecord(
		"KNetCore",
		knetpb.RecordLevel_INFO.String(),
		knetpb.RecordCode_NET.String(),
		"gRPC сервер поднят на "+knc.MyNode.ListeningIP+":"+strconv.Itoa(int(knc.MyNode.ListeningPort)),
	))

	go func() {
		if err := knc.srv.Serve(lis); err != nil {
			knc.journal.Write(KJournal.NewRecord(
				"KNetCore",
				knetpb.RecordLevel_ERROR.String(),
				knetpb.RecordCode_NET.String(),
				fmt.Sprintf("serve: %v", err),
			))
		}
	}()
}

//endregion

//region^ Хелперы остановки

func (knc *KNetCore) stopServer() {
	if knc.srv != nil {
		knc.srv.GracefulStop()
		knc.journal.Write(KJournal.NewRecord(
			"KNetCore",
			knetpb.RecordLevel_INFO.String(),
			knetpb.RecordCode_NET.String(),
			"gRPC сервер остановлен",
		))
	}
}

//endregion^
