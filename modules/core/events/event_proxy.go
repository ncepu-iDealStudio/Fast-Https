package events

func get_data_from_server() string {
	return "this is info from proxy server"
}

func ProxyEvent() string {
	return get_data_from_server()
}
