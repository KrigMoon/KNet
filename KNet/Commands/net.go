package Commands

import (
	"encoding/json"
	"fmt"

	"github.com/KrigMoon/KCommandParser"
	"github.com/KrigMoon/KNet/Proto"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

//region Connect

func ConnectHelp() string {
	return "/connect <addr> <password> — Connect to a node by address and password"
}

func ConnectCheck(i Interface, cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
	if len(cmd.Args) != 2 {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "Incorrect number of arguments"), false
	}
	addr := cmd.Args[0]
	password := cmd.Args[1]

	if addr.Addr == "" {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "The first argument must be an address: 000.000.000.000:00000"), false
	}

	if password.Str == "" {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "The second argument must be a string: \"...\""), false
	}

	return nil, true
}

func Connect(i Interface, cmd *KCommandParser.Command) *KCommandParser.Response {
	addr := cmd.Args[0].Value()
	password := cmd.Args[1].Value()

	if err := i.KNetCore.Connect(addr, nil, password); err != nil {
		i.KNet.GetJournal().Write(Proto.NewRecord(
			"KNet",
			knetpb.RecordLevel_ERROR,
			knetpb.RecordCode_CommandCenter,
			"Connection failure: "+err.Error(),
		))
		return Proto.NewResponse(
			cmd.ID,
			knetpb.RecordLevel_ERROR,
			"Connection failure: "+err.Error(),
		)
	}

	return Proto.NewResponse(
		cmd.ID,
		knetpb.RecordLevel_INFO,
		"Successful connection",
	)
}

//endregion

//region Disconnect

func DisconnectHelp() string {
	return "/disconnect <peerID> — Disconnect from a node"
}

func DisconnectCheck(i Interface, cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
	if len(cmd.Args) != 1 {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "Incorrect number of arguments"), false
	}

	id := cmd.Args[0]

	if id.ID == "" {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "The first argument must be an ID: $..."), false
	}

	return nil, true
}

func Disconnect(i Interface, cmd *KCommandParser.Command) *KCommandParser.Response {
	id := cmd.Args[0].Value()
	i.KNetCore.RemovePeer(id)

	return Proto.NewResponse(
		cmd.ID,
		knetpb.RecordLevel_INFO,
		"Successful disconnection",
	)
}

//endregion

//region Send

func SendHelp() string {
	return "/send <peerID|-b> <data> [-m <metadata>] — Send a text message (-b: broadcast)"
}

func JSONHelp() string {
	return "/json <peerID|-b> <json> [-m <metadata>] — Send a JSON message (-b: broadcast)"
}

func RemoteHelp() string {
	return "/remote <peerID|-b> <command> [-m <metadata>] — Send a remote command (-b: broadcast)"
}

func SendCheck(i Interface, cmd *KCommandParser.Command, pkgType knetpb.PackageType) (*KCommandParser.Response, bool) {
	if len(cmd.Args) != 2 && len(cmd.Args) != 4 {
		return Proto.NewResponse(
			cmd.ID,
			knetpb.RecordLevel_ERROR,
			"Incorrect number of arguments",
		), false
	}

	id := cmd.Args[0]
	data := cmd.Args[1]
	metadata := ""
	if len(cmd.Args) == 4 {
		if cmd.Args[2].Key != "-m" {
			return Proto.NewResponse(
				cmd.ID,
				knetpb.RecordLevel_ERROR,
				"Expected key: -m, got: "+cmd.Args[2].Key,
			), false
		}
		metadata = cmd.Args[3].Str
		if metadata == "" {
			return Proto.NewResponse(
				cmd.ID,
				knetpb.RecordLevel_ERROR,
				fmt.Sprintf("The metadata is specified uncorrected (%s != %s)", metadata, cmd.Args[3].Value()),
			), false
		}
	}

	if id.ID == "" && id.Key != "-b" {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "The first argument must be an ID($...) or Key(-b)"), false
	}

	if data.Str == "" {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "The second argument must be a string: \"...\""), false
	}

	if pkgType == knetpb.PackageType_JSON && !json.Valid([]byte(cmd.Args[1].Value())) {
		return Proto.NewResponse(
			cmd.ID,
			knetpb.RecordLevel_ERROR,
			"Invalid JSON",
		), false
	}

	return nil, true
}

func Send(i Interface, cmd *KCommandParser.Command, pkgType knetpb.PackageType) *KCommandParser.Response {
	to := cmd.Args[0].Value()
	if to != "-b" && !i.KNetCore.IsPeerKnown(to) {
		return Proto.NewResponse(
			cmd.ID,
			knetpb.RecordLevel_ERROR,
			fmt.Sprintf("%s: unknown peer %q", cmd.Name, to),
		)
	}

	data := cmd.Args[1].Value()
	metadata := ""
	if len(cmd.Args) == 4 {
		metadata = cmd.Args[3].Value()
	}

	send := func(peerID string) {
		i.KNetCore.Write(&knetpb.Package{
			From:     i.KNetCore.GetMyNode().Id,
			To:       peerID,
			Type:     pkgType,
			DataType: knetpb.DataType_STRING,
			Metadata: metadata,
			Payload:  &knetpb.Package_Text{Text: data},
		})
	}

	if to == "-b" {
		peers := i.KNetCore.GetPeerList()
		for _, peer := range peers {
			send(peer.Node.Id)
			for _, subPeer := range peer.PeerList {
				send(subPeer.Id)
			}
		}

	} else {
		send(to)
	}

	return Proto.NewResponse(
		cmd.ID,
		knetpb.RecordLevel_INFO,
		cmd.Name+": the packet has been sent to the node "+to,
	)
}

//endregion

//region Help

func HelpHelp() string {
	return "/help -<name> — Show help for a KNet command (e.g. /help -connect)"
}

func HelpCheck(i Interface, cmd *KCommandParser.Command) (*KCommandParser.Response, bool) {
	if len(cmd.Args) != 1 {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "Incorrect number of arguments"), false
	}

	if cmd.Args[0].Key == "" {
		return Proto.NewResponse(cmd.ID, knetpb.RecordLevel_ERROR, "The argument must be a key"), false
	}

	return nil, true
}

func Help(i Interface, cmd *KCommandParser.Command) *KCommandParser.Response {
	level := knetpb.RecordLevel_INFO
	message := ""
	cmdName := "/" + cmd.Args[0].Value()[1:]

	lc := i.KNet.GetLocalCommands()
	if cmd, ok := lc[cmdName]; ok {
		message = cmd.Help()
	} else {
		message = "/help: Unknown command " + cmdName
		level = knetpb.RecordLevel_ERROR
	}

	return Proto.NewResponse(
		cmd.ID,
		level,
		message,
	)
}

//endregion
