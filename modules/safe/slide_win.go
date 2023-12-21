package safe

type IP string

var deny []string

type FrontendDemo struct {
	Idx int
	IP  [1000]IP
}

func (fd *FrontendDemo) insert(ip string) {
	if fd.Idx >= 1000 {
		fd.Idx = 0
	}
	fd.IP[fd.Idx] = IP(ip)
	fd.Idx++
}

func (fd *FrontendDemo) check() bool {
	deny = []string{"192.168.1.1", "192.168.1.2"}
	return true
}

// func main() {
// 	fd := &FrontendDemo{}
// 	fd.insert("192.168.1.1")
// 	fd.insert("192.168.1.2")
// 	go func() {
// 		for {
// 			fd.check()
// 			time.Sleep(time.Second)
// 		}
// 	}()
// }
