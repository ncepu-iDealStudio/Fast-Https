package util

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

var util = Util{}

func init() {
	log.SetFlags(log.Lshortfile)
}

// Must Header with prefix
var MustHeader = map[string]string{
	":authority": "authority",
	":method":    "method",
	":path":      "path",
	":scheme":    "scheme",
	":status":    "status",
	// invert
	"Authority": ":authority",
	"Method":    ":method",
	"Path":      ":path",
	"Scheme":    ":scheme",
	"Status":    ":status",
}

type Util struct{}

var (
	NextClientStreamID chan uint32 = util.NextID(1)
	NextServerStreamID chan uint32 = util.NextID(2)
)

func (u Util) NextID(id uint32) chan uint32 {
	idChan := make(chan uint32)
	go func() {
		for {
			if id >= 4294967295 { // 2^32-1 or invalid
				log.Println("stream id too big or invalid, return to 0")
				id = 0
			}
			idChan <- id
			id = id + 2
		}
	}()
	return idChan
}

func RequestString(req *http.Request) string {
	str := fmt.Sprintf("%v %v %v", req.Method, req.URL, req.Proto)
	for name, value := range req.Header {
		str += fmt.Sprintf("\n%s: %s", name, strings.Join(value, ","))
	}
	return str
}

func ResponseString(res *http.Response) string {
	str := fmt.Sprintf("%v %v", res.Proto, res.Status)
	for name, value := range res.Header {
		str += fmt.Sprintf("\n%s: %s", name, strings.Join(value, ","))
	}
	return str
}

func Indent(v interface{}) string {
	return strings.Replace(fmt.Sprintf("%v", v), "\n", "\n\t\t\t\t", -1)
}
