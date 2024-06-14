package listener

func getReloadAddedListeninfo(ports []string, currli []Listener) []Listener {
	var CurrLisinfosAdded []Listener

	return CurrLisinfosAdded
}

func updateCommmon(ports []string) {

}

func updateRemoved(ports []string) {

}

func copyToNewLinster(newLis []Listener) {

}

func ReloadListenCfg() ([]Listener, []Listener, []string) {
	var NewLisinfosAll []Listener
	// new listen ports
	new_ports := FindPorts()
	old_ports := findOldPorts()
	added, removed, common := comparePorts(old_ports, new_ports)

	copyToNewLinster(NewLisinfosAll)

	updateRemoved(removed)
	updateCommmon(common)
	ListeninfoAdded := getReloadAddedListeninfo(added, NewLisinfosAll)

	Lisinfos = NewLisinfosAll
	return NewLisinfosAll, ListeninfoAdded, removed
}
