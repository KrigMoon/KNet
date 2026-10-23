package KNet

import (
	"fmt"

	"github.com/KrigMoon/KNet/KNetCore"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

type KNetMetrics struct {
	Status knetpb.Status
	Core   KNetCore.KNetCoreMetrics
}

func (knm *KNetMetrics) ToFormatedText() string {
	text := "KNetCore\n"
	text += fmt.Sprintf("    Статус: %s\n", knm.Status.String())

	text += knm.Core.ToFormatedText()

	return text
}

func (kn *KNet) GetMetrics() KNetMetrics {
	m := KNetMetrics{
		Status: kn.status,
		Core:   kn.knc.GetMetrics(),
	}

	return m
}
