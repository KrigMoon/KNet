package KNet

import (
	"fmt"

	"github.com/KrigMoon/KCommandParser"
	"github.com/KrigMoon/KJournal"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

//region Команды

type CommandContainer struct {
	Check   func(*KCommandParser.Command) (*KCommandParser.Response, bool)
	Execute func(CMDCore, *KCommandParser.Command) *KCommandParser.Response
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
				resp = container.Execute(&kn.knc, cmd)
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

//region Пакеты

func (kn *KNet) readerPKG() {
	kn.proccesses.Add(1)
	defer kn.proccesses.Done()

	for {
		select {
		case <-kn.ctx.Done():
			return
		default:
			pkg := kn.knc.Read()
			if pkg == nil {
				continue
			}

			switch pkg.Type {
			case knetpb.PackageType_COMMAND:
				kn.journal.Write(KJournal.NewRecord(
					"KNet",
					knetpb.RecordLevel_WARNING.String(),
					knetpb.RecordCode_DATA.String(),
					fmt.Sprintf("Получена команда от %s", pkg.From),
				))
				kn.Execute(pkg.Data)
			default:
				select {
				case kn.packages <- pkg:
				default:
					kn.journal.Write(KJournal.NewRecord(
						"KNet",
						knetpb.RecordLevel_WARNING.String(),
						knetpb.RecordCode_DATA.String(),
						"Буфер обработаных пакетов заполнен",
					))
				}
			}
		}
	}
}

func (kn *KNet) GetPackages() (*knetpb.Package, bool) {
	select {
	case pkg, ok := <-kn.packages:
		return pkg, ok
	default:
		return nil, false
	}
}

//endregion
