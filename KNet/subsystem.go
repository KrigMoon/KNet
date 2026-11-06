package KNet

import (
	"fmt"
	"sync"
	"time"

	"github.com/KrigMoon/KCommandParser"
	"github.com/KrigMoon/KNet/KNet/Commands"
	"github.com/KrigMoon/KNet/Proto"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
	"github.com/google/uuid"
)

//region Programs

//region^ Program preparation

func (kn *KNet) Execute(command string) error {
	// * Preparatory stage
	strategy := KCommandParser.Detect(command)
	var program *KCommandParser.Program
	switch strategy {
	case KCommandParser.InputCommand:
		cmd, err := KCommandParser.ParseCommand(command)
		if err != nil {
			return fmt.Errorf("Parse error: %s", err)
		}

		program = &KCommandParser.Program{
			Mode:  KCommandParser.ProgramModeSeq,
			Count: 1,
			Body:  []*KCommandParser.Command{cmd},
		}
	case KCommandParser.InputProgram:
		programRaw, err := KCommandParser.ParseProgram(command)
		if err != nil {
			return fmt.Errorf("Parse error: %s", err)
		}
		program = programRaw
	case KCommandParser.InputUnknown:
		return fmt.Errorf("The input is not recognized")
	}

	//* Distribution by performers and cursors formation
	if program.Mode == KCommandParser.ProgramModeSeq {
		errorBlock := ""
		for i := range program.Count {
			_ = i
			for _, cmd := range program.Body {
				if _, ok := kn.localCommandsMap[cmd.Name]; ok {
					errPaste := kn.pasteCursor(&Commands.CursorProgram{
						ID:           cmd.ID,
						Executer:     Commands.ExecuterLocal,
						Wait:         1,
						LocalProgram: []*KCommandParser.Command{cmd},
					})
					if errPaste != nil {
						return errPaste
					}
					continue
				}

				if _, ok := kn.settings.ExternalCommandsMap[cmd.Name]; ok {
					errPaste := kn.pasteCursor(&Commands.CursorProgram{
						ID:              cmd.ID,
						Executer:        Commands.ExecuterExternal,
						Wait:            1,
						ExternalProgram: []*KCommandParser.Command{cmd},
					})
					if errPaste != nil {
						return errPaste
					}
					continue
				}

				errorBlock = fmt.Sprintf("%sunknown command: %s\n", errorBlock, cmd.Name)
			}
		}
		if errorBlock != "" {
			return fmt.Errorf("%s", errorBlock)
		}
	} else {
		var loc, ext []*KCommandParser.Command
		errorBlock := ""
		for _, cmd := range program.Body {
			if _, ok := kn.localCommandsMap[cmd.Name]; ok {
				loc = append(loc, cmd)
				continue
			}

			if _, ok := kn.settings.ExternalCommandsMap[cmd.Name]; ok {
				ext = append(ext, cmd)
				continue
			}

			errorBlock = fmt.Sprintf("%sunknown command: %s\n", errorBlock, cmd.Name)
		}
		if errorBlock != "" {
			return fmt.Errorf("%s", errorBlock)
		}

		executerModeCode := 0
		if len(loc) != 0 {
			executerModeCode += 1
		}
		if len(ext) != 0 {
			executerModeCode += 2
		}
		var executer Commands.CursorExecuter
		switch executerModeCode {
		case 1:
			executer = Commands.ExecuterLocal
		case 2:
			executer = Commands.ExecuterExternal
		case 3:
			executer = Commands.ExecuterBoth
		case 0:
			return fmt.Errorf("Unknown executer")
		}

		for i := range program.Count {
			_ = i
			groupID := uuid.New().String()
			groupLoc, groupExt := Commands.CloneCommands(loc), Commands.CloneCommands(ext)

			for _, item := range groupLoc {
				item.ID = groupID
			}
			for _, item := range groupExt {
				item.ID = groupID
			}

			errPaste := kn.pasteCursor(&Commands.CursorProgram{
				ID:              groupID,
				Executer:        executer,
				Wait:            len(groupLoc) + len(groupExt),
				LocalProgram:    groupLoc,
				ExternalProgram: groupExt,
			})
			if errPaste != nil {
				return errPaste
			}
		}
	}

	return nil
}

//endregion^

//region^ Local Programs

func (kn *KNet) executerPrograms() {
	kn.processes.Add(1)
	defer kn.processes.Done()

	oldCursor := Commands.CursorNil
	for {
		select {
		case <-kn.ctx.Done():
			return
		default:
			newCursor := kn.GetCursor()
			if newCursor == Commands.CursorNil {
				select {
				case cursor := <-kn.cursorQueue:
					newCursor = cursor
					kn.cursor.Store(cursor)
				default:
					kn.cursor.Store(Commands.CursorNil)
					time.Sleep(50 * time.Millisecond)
					continue
				}
			}

			if oldCursor == newCursor {
				time.Sleep(50 * time.Millisecond)
				continue
			}

			if newCursor.Executer == Commands.ExecuterLocal || newCursor.Executer == Commands.ExecuterBoth {
				loc := newCursor.LocalProgram
				var wait sync.WaitGroup
				for _, item := range loc {
					wait.Add(1)
					go func() {
						defer wait.Done()
						container := kn.localCommandsMap[item.Name]
						resp, ok := container.Check(Commands.Interface{KNet: kn, KNetCore: &kn.knc}, item)
						if ok {
							resp = container.Body(Commands.Interface{KNet: kn, KNetCore: &kn.knc}, item)
						}
						kn.WriteResponse(resp)
					}()
				}
				wait.Wait()
			}

			oldCursor = newCursor
		}
	}
}

func (kn *KNet) pasteCursor(cursor *Commands.CursorProgram) error {
	select {
	case kn.cursorQueue <- cursor:
		return nil
	default:
		return fmt.Errorf("cursor chan busy")
	}
}

//endregion^

//region^ External Programs

func (kn *KNet) GetCursor() *Commands.CursorProgram {
	c := kn.cursor.Load().(*Commands.CursorProgram)

	if c == nil {
		return Commands.CursorNil
	}

	return c
}

func (kn *KNet) ExternalPrograms() []*KCommandParser.Command {
	cursor := kn.GetCursor()
	if cursor.Executer == Commands.ExecuterLocal {
		return nil
	}

	return cursor.ExternalProgram
}

//endregion^
//endregion

//region Response

func (kn *KNet) WriteResponse(resp *KCommandParser.Response) error {
	cursor := kn.GetCursor()
	if cursor.ID == resp.CommandID {
		cursor.Wait -= 1
		if cursor.Wait <= 0 {
			select {
			case newCursor := <-kn.cursorQueue:
				kn.cursor.Store(newCursor)
			default:
				kn.cursor.Store(Commands.CursorNil)
			}
		} else {
			kn.cursor.Store(cursor)
		}
	}
	select {
	case kn.responses <- resp:
		return nil
	default:
		return fmt.Errorf("responses chan busy")
	}
}

func (kn *KNet) Response() *KCommandParser.Response {
	select {
	case resp := <-kn.responses:
		return resp
	default:
		return nil
	}
}

//endregion

//region Packages

func (kn *KNet) routePackages() {
	kn.processes.Add(1)
	defer kn.processes.Done()

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
				if kn.settings.RemoteControl {
					kn.settings.Journal.Write(Proto.NewRecord(
						"KNet",
						knetpb.RecordLevel_WARNING,
						knetpb.RecordCode_DATA,
						fmt.Sprintf("Received a command from %s", pkg.From),
					))
					if pkg.DataType == knetpb.DataType_STRING {
						kn.Execute(pkg.GetText())
					}
				}
			default:
				kn.readers.Range(func(_, v any) bool {
					ch := v.(ReaderPackages)
					if ch.Rules(pkg) {
						select {
						case ch.Chan <- pkg:
						default:
							kn.settings.Journal.Write(Proto.NewRecord(
								"KNet",
								knetpb.RecordLevel_WARNING,
								knetpb.RecordCode_DATA,
								"The packet buffer is full",
							))

						}
					}
					return true
				})
			}
		}
	}
}

//region Readers

type ReaderPackages struct {
	Chan  chan *knetpb.Package
	Rules func(*knetpb.Package) bool
}

func (kn *KNet) AddReaderPackages(id *string, buff int, rules func(*knetpb.Package) bool) string {
	if rules == nil {
		rules = func(r *knetpb.Package) bool { return true }
	}
	var ID string
	if id == nil {
		ID = uuid.NewString()
	} else {
		ID = *id
	}
	if _, ok := kn.readers.Load(ID); ok {
		return ID
	}

	reader := ReaderPackages{Chan: make(chan *knetpb.Package, buff), Rules: rules}
	kn.readers.Store(ID, reader)

	kn.readersWait.Add(1)
	kn.readersCount.Add(1)

	return ID
}

func (kn *KNet) DelReaderPackages(id string) {
	kn.readers.Delete(id)
	kn.readersCount.Add(-1)
	kn.readersWait.Done()
}

//endregion

func (kn *KNet) Package(id string) *knetpb.Package {
	v, ok := kn.readers.Load(id)
	if !ok {
		return nil
	}
	ch := v.(ReaderPackages)

	select {
	case pkg := <-ch.Chan:
		return pkg
	default:
		return nil
	}
}

//endregion
