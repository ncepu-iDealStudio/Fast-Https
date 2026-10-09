package listener

import (
	"context"
	"fast-https/utils/logger"
)

func getReloadAddedListeninfo(ports []string, currli *[]Listener) []Listener {
	var CurrLisinfosAdded []Listener
	SortBySpecificPorts(ports, &CurrLisinfosAdded)

	processListenData(&CurrLisinfosAdded)
	processHostMap(&CurrLisinfosAdded)

	for index, each := range CurrLisinfosAdded {
		if each.LisType == 1 || each.LisType == 10 {
			CurrLisinfosAdded[index].certs = newCertSet()
			CurrLisinfosAdded[index].Lfd = listenSsl("0.0.0.0:"+each.Port, CurrLisinfosAdded[index].certs, each.Cfg, true)
		} else {
			CurrLisinfosAdded[index].Lfd = listenTcp("0.0.0.0:"+each.Port, true)
		}
		// added 端口需要新的 Ctx/Cancel
		ctx, cancel := context.WithCancel(context.Background())
		CurrLisinfosAdded[index].Ctx = ctx
		CurrLisinfosAdded[index].Cancel = cancel
		logger.Debug("server current listen info added: %s", each.Port)
	}

	*currli = append(*currli, CurrLisinfosAdded...)
	return CurrLisinfosAdded
}

func updateCommonToNewLinster(common_ports []string, newLis *[]Listener) (removeOverlap []string) {
	var CurrLisinfoCommon []Listener

	// sort by port
	SortBySpecificPorts(common_ports, &CurrLisinfoCommon)
	processListenData(&CurrLisinfoCommon)
	processHostMap(&CurrLisinfoCommon)
	// fill cfg

	for index, each := range CurrLisinfoCommon {
		for i, old := range GLisinfos {
			if old.Port == each.Port {
				if old.LisType == each.LisType {
					// 复用旧的 Ctx/Cancel/Lfd，配置已通过 processListenData/processHostMap 生成
					CurrLisinfoCommon[index].Lfd = old.Lfd
					CurrLisinfoCommon[index].Ctx = old.Ctx
					CurrLisinfoCommon[index].Cancel = old.Cancel
					CurrLisinfoCommon[index].certs = old.certs

					if (each.LisType == 1 || each.LisType == 10) && old.certs != nil {
						if err := old.certs.replace(each.Cfg); err != nil {
							logger.Error("reload kept previous certificate: %s", err.Error())
						}
					}

					// 原地更新旧 Listener 的 Cfg/HostMap
					// 此时 GLisinfos 与 s.Listens 共享底层数组
					// 正在运行的 serveListener 通过 &s.Listens[i] 持有指针，能读到新配置
					GLisinfos[i].Cfg = each.Cfg
					GLisinfos[i].HostMap = each.HostMap
					GLisinfos[i].certs = old.certs
				} else {
					logger.Debug("port %s listen type changed", each.Port)
					removeOverlap = append(removeOverlap, each.Port)
					if each.LisType == 1 || each.LisType == 10 {
						old.Lfd.Close()
						CurrLisinfoCommon[index].certs = newCertSet()
						CurrLisinfoCommon[index].Lfd = listenSsl("0.0.0.0:"+each.Port, CurrLisinfoCommon[index].certs, each.Cfg, true)
					} else {
						old.Lfd.Close()
						CurrLisinfoCommon[index].Lfd = listenTcp("0.0.0.0:"+each.Port, true)
					}
					// 类型变化时创建新的 Ctx/Cancel（旧的需要被 cancel 让 serveListener 退出）
					ctx, cancel := context.WithCancel(context.Background())
					CurrLisinfoCommon[index].Ctx = ctx
					CurrLisinfoCommon[index].Cancel = cancel
				}
				logger.Debug("update: %s", each.Port)
				break
			}
		}
	}

	*newLis = append(*newLis, CurrLisinfoCommon...)

	return
}

func ReloadListenCfg() ([]Listener, []Listener, []string) {
	var NewLisinfosAll []Listener
	// new listen ports
	new_ports := FindPorts()
	old_ports := FindOldPorts()
	added, removed, common := comparePorts(old_ports, new_ports)

	updateCommonToNewLinster(common, &NewLisinfosAll)
	// removeOverlap := updateCommonToNewLinster(common, &NewLisinfosAll)
	// removed = append(removed, removeOverlap...)
	// added = append(added, removeOverlap...)
	ListeninfoAdded := getReloadAddedListeninfo(added, &NewLisinfosAll)

	GLisinfos = NewLisinfosAll
	return NewLisinfosAll, ListeninfoAdded, removed
}
