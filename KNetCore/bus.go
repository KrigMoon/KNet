package KNetCore

import (
	"github.com/KrigMoon/KJournal"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

func (knc *KNetCore) Write(pkg *knetpb.Package) {
	select {
	case knc.entryPackages <- pkg:
	default:
		knc.journal.Write(KJournal.NewRecord(
			"KNetCore",
			knetpb.RecordLevel_WARNING.String(),
			knetpb.RecordCode_DATA.String(),
			"entry bus busy",
		))
	}
}

func (knc *KNetCore) Read() *knetpb.Package {
	select {
	case pkg := <-knc.exitPackages:
		return pkg
	default:
		return nil
	}
}
