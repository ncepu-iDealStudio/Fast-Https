package listener

import "fast-https/utils/logger"

func getReloadAddedListeninfo(ports []string, currli *[]Listener) []Listener {
	var CurrLisinfosAdded []Listener
	SortBySpecificPorts(ports, &CurrLisinfosAdded)

	processListenData(&CurrLisinfosAdded)
	processHostMap(&CurrLisinfosAdded)

	for index, each := range CurrLisinfosAdded {
		if each.LisType == 1 || each.LisType == 10 {
			CurrLisinfosAdded[index].Lfd = listenSsl("0.0.0.0:"+each.Port, each.Cfg)
		} else {
			CurrLisinfosAdded[index].Lfd = listenTcp("0.0.0.0:" + each.Port)
		}
		logger.Debug("server current listen info added: %s", each.Port)
	}

	*currli = append(*currli, CurrLisinfosAdded...)
	return CurrLisinfosAdded
}

func updateCommonToNewLinster(ports []string, newLis *[]Listener) {
	var CurrLisinfoCommon []Listener

	// sort by port
	SortBySpecificPorts(ports, &CurrLisinfoCommon)
	processListenData(&CurrLisinfoCommon)
	processHostMap(&CurrLisinfoCommon)
	// fill cfg

	*newLis = append(*newLis, CurrLisinfoCommon...)
}

func ReloadListenCfg() ([]Listener, []Listener, []string) {
	var NewLisinfosAll []Listener
	// new listen ports
	new_ports := FindPorts()
	old_ports := findOldPorts()
	added, removed, common := comparePorts(old_ports, new_ports)

	updateCommonToNewLinster(common, &NewLisinfosAll)

	ListeninfoAdded := getReloadAddedListeninfo(added, &NewLisinfosAll)

	GLisinfos = NewLisinfosAll
	return NewLisinfosAll, ListeninfoAdded, removed
}
