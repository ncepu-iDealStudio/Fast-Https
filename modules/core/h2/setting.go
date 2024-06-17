package h2

import (
	"fast-https/modules/core/h2/frame"
)

const (
	OVER_TLS           string = "h2"
	OVER_TCP           string = "h2c"
	VERSION            string = OVER_TLS
	CONNECTION_PREFACE string = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
)

var DefaultSettings = map[frame.SettingsID]int32{
	frame.SETTINGS_HEADER_TABLE_SIZE: frame.DEFAULT_HEADER_TABLE_SIZE,
	// SETTINGS_ENABLE_PUSH:            DEFAULT_ENABLE_PUSH, // server dosen't send this
	frame.SETTINGS_MAX_CONCURRENT_STREAMS: frame.DEFAULT_MAX_CONCURRENT_STREAMS,
	frame.SETTINGS_INITIAL_WINDOW_SIZE:    frame.DEFAULT_INITIAL_WINDOW_SIZE,
	frame.SETTINGS_MAX_FRAME_SIZE:         frame.DEFAULT_MAX_FRAME_SIZE,
	frame.SETTINGS_MAX_HEADER_LIST_SIZE:   frame.DEFAULT_MAX_HEADER_LIST_SIZE,
}

var NilSettings = make(map[frame.SettingsID]int32, 0)
