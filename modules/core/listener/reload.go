package listener

func getReloadAddedListeninfo(ports []string, currli []Listener) []Listener {
	var CurrLisinfosAdded []Listener
	SortBySpecificPorts(ports, CurrLisinfosAdded)

	processListenData(CurrLisinfosAdded)
	processHostMap(CurrLisinfosAdded)

	_ = append(currli, CurrLisinfosAdded...)
	return CurrLisinfosAdded
}

func updateCommonToNewLinster(ports []string, newLis []Listener) {
	var CurrLisinfoCommon []Listener

	// sort by port
	SortBySpecificPorts(ports, CurrLisinfoCommon)
	processListenData(CurrLisinfoCommon)
	processHostMap(CurrLisinfoCommon)
	// fill cfg

	_ = append(newLis, CurrLisinfoCommon...)
}

func ReloadListenCfg() ([]Listener, []Listener, []string) {
	var NewLisinfosAll []Listener
	// new listen ports
	new_ports := FindPorts()
	old_ports := findOldPorts()
	added, removed, common := comparePorts(old_ports, new_ports)

	updateCommonToNewLinster(common, NewLisinfosAll)

	ListeninfoAdded := getReloadAddedListeninfo(added, NewLisinfosAll)

	GLisinfos = NewLisinfosAll
	return NewLisinfosAll, ListeninfoAdded, removed
}
