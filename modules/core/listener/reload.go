package listener

import "fast-https/utils/logger"

func getReloadAddedListeninfo(ports []string, currli *[]Listener) []Listener {
	var CurrLisinfosAdded []Listener
	SortBySpecificPorts(ports, &CurrLisinfosAdded)

	processListenData(&CurrLisinfosAdded)
	processHostMap(&CurrLisinfosAdded)

	for index, each := range CurrLisinfosAdded {
		if each.LisType == 1 || each.LisType == 10 {
			CurrLisinfosAdded[index].Lfd = listenSsl("0.0.0.0:"+each.Port, each.Cfg, true)
		} else {
			CurrLisinfosAdded[index].Lfd = listenTcp("0.0.0.0:"+each.Port, true)
		}
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
		for _, old := range GLisinfos {
			if old.Port == each.Port {
				if old.LisType == each.LisType {
					CurrLisinfoCommon[index].Lfd = old.Lfd
				} else {
					logger.Debug("port %s listen type changed", each.Port)
					removeOverlap = append(removeOverlap, each.Port)
					if each.LisType == 1 || each.LisType == 10 {
						old.Lfd.Close()
						CurrLisinfoCommon[index].Lfd = listenSsl("0.0.0.0:"+each.Port, each.Cfg, true)
					} else {
						old.Lfd.Close()
						CurrLisinfoCommon[index].Lfd = listenTcp("0.0.0.0:"+each.Port, true)
					}
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
