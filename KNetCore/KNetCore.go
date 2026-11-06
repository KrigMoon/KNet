package KNetCore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/KrigMoon/KJournal"
	"github.com/KrigMoon/KNet/Proto"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
	"github.com/admpub/once"
	"google.golang.org/grpc"
)

//region Peer

type Peer struct {
	Node   *knetpb.Node
	Client knetpb.NetNodeClient
	Conn   *grpc.ClientConn
}

type PeerInfo struct {
	Node     *knetpb.Node
	PeerList []*knetpb.Node
}

//endregion

//region Structure, settings and constructor

//region^ KNetCore's Settings

type KNCSettings struct {
	Journal  *KJournal.KJournal
	MyNode   *knetpb.Node
	Password string
}

// desc: Validates settings
func (kncs *KNCSettings) validate() error {
	if kncs.Journal == nil {
		return fmt.Errorf("KNCSettings: Journal is required")
	}
	if kncs.MyNode == nil {
		return fmt.Errorf("KNCSettings: MyNode is required")
	}
	if kncs.Password == "" {
		return fmt.Errorf("KNCSettings: Password is required")
	}
	return nil
}

// desc: Normalizing data and setting default values
func (kncs *KNCSettings) normalize() {
	sum := sha256.Sum256([]byte(kncs.Password))
	kncs.Password = hex.EncodeToString(sum[:])
}

//endregion^

//region^ KNetCore

type KNetCore struct {
	knetpb.UnimplementedNetNodeServer
	//* Settings and status
	settings KNCSettings
	status   knetpb.Status
	//* Lifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	awaitStop chan struct{}
	stopOnce  once.Once
	processes sync.WaitGroup
	//* Pipelines
	entry chan *knetpb.Package
	exit  chan *knetpb.Package
	//* gRPC and Node Database
	srv   *grpc.Server
	peers sync.Map
}

func NewKNetCore() KNetCore {
	return KNetCore{}
}

//endregion^

//endregion

//region Lifecycle

func (knc *KNetCore) Run(settings KNCSettings) error {
	if knc.status != knetpb.Status_OFF {
		return fmt.Errorf("KNetCore is already running")
	}

	//* Initialization and check settings
	knc.settings = settings
	if err := knc.settings.validate(); err != nil {
		return err
	}
	knc.settings.normalize()

	knc.settings.Journal.NewWriter()

	knc.settings.Journal.Write(Proto.NewRecord("KNetCore", knetpb.RecordLevel_INFO, knetpb.RecordCode_LIFECIRCLE, "KNetCore RUNNING"))
	knc.status = knetpb.Status_RUNNING

	//* Initialization lifecycle
	knc.ctx, knc.cancel = context.WithCancel(context.Background())
	knc.awaitStop = make(chan struct{})
	knc.stopOnce.Reset()

	//* Initialization pipelines
	knc.entry = make(chan *knetpb.Package, 256)
	knc.exit = make(chan *knetpb.Package, 256)

	//* Launch of the shutdown mechanism
	go knc.stop()

	knc.runServer()

	knc.settings.Journal.Write(Proto.NewRecord("KNetCore", knetpb.RecordLevel_INFO, knetpb.RecordCode_LIFECIRCLE, "KNetCore ON"))
	knc.status = knetpb.Status_ON

	go knc.sender()

	return nil
}

func (knc *KNetCore) Endwork() error {
	if knc.status != knetpb.Status_ON {
		return fmt.Errorf("KNetCore is already stopped")
	}

	knc.stopOnce.Do(func() {
		knc.cancel()
		<-knc.awaitStop
	})

	return nil
}

func (knc *KNetCore) stop() {
	<-knc.ctx.Done()
	knc.settings.Journal.Write(Proto.NewRecord("KNetCore", knetpb.RecordLevel_INFO, knetpb.RecordCode_LIFECIRCLE, "KNetCore STOP"))
	knc.status = knetpb.Status_STOP

	var ids []string
	knc.peers.Range(func(key, _ any) bool {
		ids = append(ids, key.(string))
		return true
	})
	for _, id := range ids {
		knc.RemovePeer(id)
	}

	knc.processes.Wait()

	knc.stopServer()

	knc.settings.Journal.Write(Proto.NewRecord("KNetCore", knetpb.RecordLevel_INFO, knetpb.RecordCode_LIFECIRCLE, "KNetCore OFF"))
	knc.status = knetpb.Status_OFF

	knc.settings.Journal.DelWriter()

	close(knc.awaitStop)
}

//region^ Helpers

func (knc *KNetCore) runServer() {
	if knc.settings.MyNode.ListeningIP == "" {
		knc.settings.MyNode.ListeningIP = localIP()
	}

	addr := net.JoinHostPort(knc.settings.MyNode.ListeningIP, strconv.Itoa(int(knc.settings.MyNode.ListeningPort)))

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		knc.settings.Journal.Write(Proto.NewRecord(
			"KNetCore",
			knetpb.RecordLevel_ERROR,
			knetpb.RecordCode_NET,
			fmt.Sprintf("listen: %v", err),
		))
	}

	if knc.settings.MyNode.ListeningPort == 0 {
		if tcpAddr, ok := lis.Addr().(*net.TCPAddr); ok {
			knc.settings.MyNode.ListeningPort = int32(tcpAddr.Port)
		}
	}

	knc.srv = grpc.NewServer()
	knetpb.RegisterNetNodeServer(knc.srv, knc)

	knc.settings.Journal.Write(Proto.NewRecord(
		"KNetCore",
		knetpb.RecordLevel_INFO,
		knetpb.RecordCode_NET,
		"The gRPC server has been raised to "+knc.settings.MyNode.ListeningIP+":"+strconv.Itoa(int(knc.settings.MyNode.ListeningPort)),
	))

	go func() {
		if err := knc.srv.Serve(lis); err != nil {
			knc.settings.Journal.Write(Proto.NewRecord(
				"KNetCore",
				knetpb.RecordLevel_ERROR,
				knetpb.RecordCode_NET,
				fmt.Sprintf("serve: %v", err),
			))
		}
	}()
}

func (knc *KNetCore) stopServer() {
	if knc.srv != nil {
		knc.srv.GracefulStop()
		knc.settings.Journal.Write(Proto.NewRecord(
			"KNetCore",
			knetpb.RecordLevel_INFO,
			knetpb.RecordCode_NET,
			"gRPC server is stopped",
		))
	}
}

//endregion^

//endregion

//region Read/Write

func (knc *KNetCore) Read() *knetpb.Package {
	select {
	case pkg := <-knc.exit:
		return pkg
	default:
		return nil
	}
}

func (knc *KNetCore) Write(pkg *knetpb.Package) {
	select {
	case knc.entry <- pkg:
	default:
		knc.settings.Journal.Write(Proto.NewRecord(
			"KNetCore",
			knetpb.RecordLevel_WARNING,
			knetpb.RecordCode_DATA,
			"entry bus busy",
		))
	}
}

//endregion

//region Processes

func (knc *KNetCore) sender() {
	knc.processes.Add(1)
	defer knc.processes.Done()

	for {
		select {
		case <-knc.ctx.Done():
			return
		case pkg := <-knc.entry:
			p := knc.findPeer(pkg.To)
			if p == nil {
				knc.settings.Journal.Write(Proto.NewRecord(
					"KNetCore",
					knetpb.RecordLevel_ERROR,
					knetpb.RecordCode_NET,
					"Recipient "+pkg.To+" not found"),
				)
				continue
			}
			if ack, err := p.Client.Send(context.Background(), pkg); err != nil {
				knc.settings.Journal.Write(Proto.NewRecord(
					"KNetCore",
					knetpb.RecordLevel_ERROR,
					knetpb.RecordCode_NET, "Sending error to the node "+pkg.To+": "+err.Error()),
				)
			} else if !ack.Ok {
				knc.settings.Journal.Write(Proto.NewRecord(
					"KNetCore",
					knetpb.RecordLevel_ERROR,
					knetpb.RecordCode_NET,
					"Sending error to the node "+pkg.To+": "+ack.Error),
				)
			}
		}
	}
}

//endregion
