package Proto

import (
	"github.com/KrigMoon/KCommandParser"
	"github.com/KrigMoon/KJournal"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

func NewRecord(author string, level knetpb.RecordLevel, code knetpb.RecordCode, message string) *KJournal.Record {
	return KJournal.NewRecord(author, level.String(), code.String(), message)
}

func NewResponse(commandID string, level knetpb.RecordLevel, responseMessage string) *KCommandParser.Response {
	return KCommandParser.NewResponse(commandID, level.String(), responseMessage)
}
