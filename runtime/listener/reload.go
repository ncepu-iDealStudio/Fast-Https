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
		each.openListener()
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
//
// 另有一种必须重建的情况：tls.Config 的 NextProtos 建好后改不了，
// 而它取决于建监听器时是否注册了 ACME 的 acme-tls/1 provider。开关
// tls-alpn-01 后若继续复用旧 *Listener，新配置永远拿不到挑战证书。
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
			switch {
			case !old.needsRebuild(each):
				// 复用旧的 Ctx/Cancel/Lfd/certs，配置整体热更新并发布
				CurrLisinfoCommon[index] = old
				old.updateCfg(each.Cfg, each.HostMap)

				if (each.LisType == 1 || each.LisType == 10) && old.certs != nil {
					if err := old.certs.replace(each.Cfg); err != nil {
						logging.ConsoleLog.Error().Msgf("reload kept previous certificate: %s", err.Error())
					}
				}
			case old.LisType != each.LisType:
				logging.ConsoleLog.Debug().Msgf("port %s listen type changed", each.Port)
				old.Lfd.Close()
				each.openListener()
				each.replaceCtx()
				typeChanged = append(typeChanged, each)
			default:
				// 只有 acme-tls/1 的通告位变了。NextProtos 建好就改不了，
				// 只能换一个 tls.Config，因此走与类型变化相同的顺序：
				// 先关旧 socket 再绑新 socket——SO_REUSEPORT 只在 linux/amd64
				// 的 ReuseCallBack 里设置，其余平台同时绑同一端口会失败。
				logging.ConsoleLog.Info().Msgf("port %s rebuilds TLS config: acme-tls/1 advertisement changed", each.Port)
				old.Lfd.Close()
				each.openListener()
				each.replaceCtx()
				typeChanged = append(typeChanged, each)
			}
			logging.ConsoleLog.Debug().Msgf("update: %s", each.Port)
			break
		}
	}

	*newLis = append(*newLis, CurrLisinfoCommon...)

	return
}

// replaceCtx 给重建出来的监听器换一套新的 Ctx/Cancel 并发布配置快照。
// 旧监听器的 Cancel 由 server.Reload 按 removed 列表调用。
func (l *Listener) replaceCtx() {
	ctx, cancel := context.WithCancel(context.Background())
	l.Ctx = ctx
	l.Cancel = cancel
	l.publish()
}

// ReloadCerts 重新读取所有 ssl 监听器当前配置里的证书文件，监听 socket
// 保持不变。控制面签发或续期证书后覆盖 PEM，再调用它完成热替换；
// 任一端口读取失败都保留该端口原来的证书，不让半截 PEM 上生产。
func ReloadCerts() error {
	// 拿快照而不是直接遍历 GLisinfos：本函数由控制面的证书协程调用，
	// 与 reload 协程替换监听器表并发。
	listeners := SnapshotListeners()
	var firstErr error
	for _, lis := range listeners {
		if lis == nil || lis.certs == nil {
			continue
		}
		if lis.LisType != 1 && lis.LisType != 10 {
			continue
		}
		cfgs := lis.Cfgs()
		if len(cfgs) == 0 {
			continue
		}
		if err := lis.certs.replace(cfgs); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			logging.ConsoleLog.Error().Msgf("reload certificates on port %s kept previous certificate: %s", lis.Port, err.Error())
			continue
		}
		logging.ConsoleLog.Info().Msgf("certificates reloaded on port %s", lis.Port)
	}
	return firstErr
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

	setListeners(NewLisinfosAll)
	return NewLisinfosAll, ListeninfoAdded, removed
}
