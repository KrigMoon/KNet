package KNet

import (
	"fmt"

	"github.com/KrigMoon/KCommandParser"
)

//region Команды

type CommandContainer struct {
	Check   func(*KCommandParser.Command) (*KCommandParser.Response, bool)
	Execute func(*KCommandParser.Command) *KCommandParser.Response
}

func (kn *KNet) Execute(command string) error {
	cmd, err := KCommandParser.ParseCommand(command)
	if err != nil {
		return err
	}

	if _, ok := kn.localCommandsMap[cmd.Name]; ok {
		select {
		case kn.commands <- cmd:
			return nil
		default:
			return fmt.Errorf("local chan busy")
		}
	}

	if _, ok := kn.externalCommandsMap[cmd.Name]; ok {
		select {
		case kn.externalCommands <- cmd:
			return nil
		default:
			return fmt.Errorf("external chan busy")
		}
	}

	return fmt.Errorf("unknown command")
}

//region^ Исполнение

// Внутренее
func (kn *KNet) executer() {
	kn.proccesses.Add(1)
	defer kn.proccesses.Done()

	for {
		select {
		case <-kn.ctx.Done():
			return
		case cmd := <-kn.commands:
			container := kn.localCommandsMap[cmd.Name]
			resp, ok := container.Check(cmd)
			if ok {
				resp = container.Execute(cmd)
			}
			kn.WriteResponse(resp)
		}
	}
}

// Внешнее
func (kn *KNet) GetCommand() (*KCommandParser.Command, bool) {
	select {
	case cmd, ok := <-kn.externalCommands:
		return cmd, ok
	default:
		return nil, false
	}
}

//endregion^

//endregion

//region Ответы

func (kn *KNet) WriteResponse(response *KCommandParser.Response) error {
	select {
	case kn.responses <- response:
		return nil
	default:
		return fmt.Errorf("response chan busy")
	}
}

func (kn *KNet) GetResponse() (*KCommandParser.Response, bool) {
	select {
	case req, ok := <-kn.responses:
		return req, ok
	default:
		return nil, false
	}
}

//endregion
