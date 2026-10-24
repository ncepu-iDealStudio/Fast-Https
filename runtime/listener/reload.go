package listener

import (
	"context"
	"fast-https/infra/logging"
)

func getReloadAddedListeninfo(ports []string, currli *[]*Listener) []*Listener {
	var CurrLisinfosAdded []*Listener
	SortBySpecificPorts(ports, &CurrLisinfosAdded)

	processListenData(&CurrLisinfosAdded)
	processHostMap(&CurrLisinfosAdded)

	for _, each := range CurrLisinfosAdded {
		if each.LisType == 1 || each.LisType == 10 {
			each.certs = newCertSet()
			each.Lfd = listenSsl("0.0.0.0:"+each.Port, each.certs, each.Cfg, each.LisType == 10, true)
		} else {
			each.Lfd = listenTcp("0.0.0.0:"+each.Port, true)
		}
		// added 端口需要新的 Ctx/Cancel
		ctx, cancel := context.WithCancel(context.Background())
		each.Ctx = ctx
		each.Cancel = cancel
		each.publish()
		logging.ConsoleLog.Debug().Msgf("server current listen info added: %s", each.Port)
	}

	*currli = append(*currli, CurrLisinfosAdded...)
	return CurrLisinfosAdded
}

// updateCommonToNewLinster 处理新旧配置都存在的端口。
//
// 类型不变时直接复用旧 *Listener（正在服务的 serveListener 与在途连接
// 持有同一指针），通过 updateCfg 发布新配置快照；
// 类型变化时关闭旧监听器（其 serveListener 以 ErrClosed 退出），
// 新建的监听器返回给调用方，随新增端口一起启动。
func updateCommonToNewLinster(common_ports []string, newLis *[]*Listener) (typeChanged []*Listener) {
	var CurrLisinfoCommon []*Listener

	// sort by port
	SortBySpecificPorts(common_ports, &CurrLisinfoCommon)
	processListenData(&CurrLisinfoCommon)
	processHostMap(&CurrLisinfoCommon)
	// fill cfg

	for index, each := range CurrLisinfoCommon {
		for _, old := range GLisinfos {
			if old.Port != each.Port {
				continue
			}
			if old.LisType == each.LisType {
				// 复用旧的 Ctx/Cancel/Lfd/certs，配置整体热更新并发布
				CurrLisinfoCommon[index] = old
				old.updateCfg(each.Cfg, each.HostMap)

				if (each.LisType == 1 || each.LisType == 10) && old.certs != nil {
					if err := old.certs.replace(each.Cfg); err != nil {
						logging.ConsoleLog.Error().Msgf("reload kept previous certificate: %s", err.Error())
					}
				}
			} else {
				logging.ConsoleLog.Debug().Msgf("port %s listen type changed", each.Port)
				old.Lfd.Close()
				if each.LisType == 1 || each.LisType == 10 {
					each.certs = newCertSet()
					each.Lfd = listenSsl("0.0.0.0:"+each.Port, each.certs, each.Cfg, each.LisType == 10, true)
				} else {
					each.Lfd = listenTcp("0.0.0.0:"+each.Port, true)
				}
				ctx, cancel := context.WithCancel(context.Background())
				each.Ctx = ctx
				each.Cancel = cancel
				each.publish()
				typeChanged = append(typeChanged, each)
			}
			logging.ConsoleLog.Debug().Msgf("update: %s", each.Port)
			break
		}
	}

	*newLis = append(*newLis, CurrLisinfoCommon...)

	return
}

func ReloadListenCfg() ([]*Listener, []*Listener, []string) {
	var NewLisinfosAll []*Listener
	// new listen ports
	new_ports := FindPorts()
	old_ports := FindOldPorts()
	added, removed, common := comparePorts(old_ports, new_ports)

	typeChanged := updateCommonToNewLinster(common, &NewLisinfosAll)
	ListeninfoAdded := getReloadAddedListeninfo(added, &NewLisinfosAll)
	// 类型变化的端口按"新增"启动新监听器，按"移除"关闭旧监听器
	ListeninfoAdded = append(ListeninfoAdded, typeChanged...)
	for _, lis := range typeChanged {
		removed = append(removed, lis.Port)
	}

	GLisinfos = NewLisinfosAll
	return NewLisinfosAll, ListeninfoAdded, removed
}
