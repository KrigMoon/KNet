package KNetCore

import (
	"fmt"
	"net"

	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

type KNetCoreMetrics struct {
	Status knetpb.Status
}

func (kncm *KNetCoreMetrics) ToFormatedText() string {
	text := "KNetCore\n"
	text += fmt.Sprintf("    Статус: %s\n", kncm.Status.String())

	return text
}

func (knc *KNetCore) GetMetrics() KNetCoreMetrics {
	m := KNetCoreMetrics{
		Status: knc.status,
	}

	return m
}

//region Служебные методы и Геттеры

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "0.0.0.0"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return "0.0.0.0"
}

func (knc *KNetCore) GetMyNode() *knetpb.Node {
	return knc.MyNode
}

//endregion
