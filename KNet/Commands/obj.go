package Commands

import (
	"time"

	"github.com/KrigMoon/KCommandParser"
	"github.com/KrigMoon/KJournal"
	"github.com/KrigMoon/KNet/KNetCore"
	"github.com/KrigMoon/KNet/Proto"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

//region Commands KNet

type CommandContainer struct {
	Help  func() string
	Check func(Interface, *KCommandParser.Command) (*KCommandParser.Response, bool)
	Body  func(Interface, *KCommandParser.Command) *KCommandParser.Response
}

func CloneCommands(src []*KCommandParser.Command) []*KCommandParser.Command {
	if len(src) == 0 {
		return nil
	}
	dst := make([]*KCommandParser.Command, 0, len(src))
	for _, cmd := range src {
		if cmd == nil {
			continue
		}
		c := *cmd
		dst = append(dst, &c)
	}
	return dst
}

type CommandsSettings struct {
	FileStorageDir string
}

func GetCommandsMap(settings CommandsSettings) map[string]CommandContainer {
	return map[string]CommandContainer{
		"/connect": {
			Help:  ConnectHelp,
			Check: ConnectCheck,
			Body:  Connect,
		},
		"/disconnect": {
			Help:  DisconnectHelp,
			Check: DisconnectCheck,
			Body:  Disconnect,
		},
		"/send": {
			Help: SendHelp,
			Check: func(i Interface, cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
				return SendCheck(i, cmd, knetpb.PackageType_MESSAGE)
			},
			Body: func(i Interface, cmd *KCommandParser.Command) *KCommandParser.Response {
				return Send(i, cmd, knetpb.PackageType_MESSAGE)
			},
		},
		"/json": {
			Help: JSONHelp,
			Check: func(i Interface, cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
				return SendCheck(i, cmd, knetpb.PackageType_JSON)
			},
			Body: func(i Interface, cmd *KCommandParser.Command) *KCommandParser.Response {
				return Send(i, cmd, knetpb.PackageType_JSON)
			},
		},
		"/remote": {
			Help: RemoteHelp,
			Check: func(i Interface, cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
				return SendCheck(i, cmd, knetpb.PackageType_COMMAND)
			},
			Body: func(i Interface, cmd *KCommandParser.Command) *KCommandParser.Response {
				return Send(i, cmd, knetpb.PackageType_COMMAND)
			},
		},
		"/file": {
			Help:  FileHelp,
			Check: FileCheck,
			Body: func(i Interface, cmd *KCommandParser.Command) *KCommandParser.Response {
				return File(i, cmd, settings.FileStorageDir)
			},
		},
		"/help": {
			Help:  HelpHelp,
			Check: HelpCheck,
			Body:  Help,
		},
		"/sleep": {
			Help: func() string {
				return "/sleep <seconds> — pause execution for N seconds."
			},
			Check: func(i Interface, c *KCommandParser.Command) (*KCommandParser.Response, bool) {
				if len(c.Args) != 1 {
					return Proto.NewResponse(c.ID, knetpb.RecordLevel_ERROR, "Incorrect number of arguments"), false
				}

				if c.Args[0].Number == "" {
					return Proto.NewResponse(c.ID, knetpb.RecordLevel_ERROR, "The first argument must be an Number: 15;45;34"), false
				}

				return nil, true
			},
			Body: func(i Interface, c *KCommandParser.Command) *KCommandParser.Response {
				seconds := int(*c.Args[0].ToFloat64())
				time.Sleep(time.Duration(seconds) * time.Second)
				return Proto.NewResponse(c.ID, knetpb.RecordLevel_INFO, "")
			},
		},
	}
}

//endregion

//region Interfaces

type Interface struct {
	KNet     InterfaceKNet
	KNetCore InterfaceKNetCore
}

type InterfaceKNet interface {
	Execute(command string) error
	GetCursor() *CursorProgram
	GetJournal() *KJournal.KJournal
	GetLocalCommands() map[string]CommandContainer
	AddReaderPackages(id *string, buff int, rules func(*knetpb.Package) bool) string
	DelReaderPackages(id string)
	Package(id string) *knetpb.Package
}

type InterfaceKNetCore interface {
	Connect(remoteAddr string, node *knetpb.Node, password string) error
	RemovePeer(id string)
	Write(pkg *knetpb.Package)
	GetJournal() *KJournal.KJournal
	GetMyNode() *knetpb.Node
	GetPeerList() []KNetCore.PeerInfo
	IsPeerKnown(id string) bool
}

//endregion

//region Cursor

type CursorExecuter int

const (
	ExecuterLocal CursorExecuter = iota
	ExecuterExternal
	ExecuterBoth
)

type CursorProgram struct {
	ID              string
	Executer        CursorExecuter
	Wait            int
	LocalProgram    []*KCommandParser.Command
	ExternalProgram []*KCommandParser.Command
}

var CursorNil *CursorProgram = &CursorProgram{
	ID:              "",
	Executer:        ExecuterLocal,
	Wait:            0,
	LocalProgram:    []*KCommandParser.Command{},
	ExternalProgram: []*KCommandParser.Command{},
}

//endregion
