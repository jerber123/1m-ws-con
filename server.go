/*
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
By https://github.com/eranyanay/1m-go-websockets
*/
package main

import (
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"syscall"
	//comienza epoll.go
	"golang.org/x/sys/unix"
	//"log"
	"net"
	"reflect"
	"sync"
	//"syscall"
	//termina epoll.go
	//"fmt"
	"strings"
)

var epoller *epoll  
func wsHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade connection
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil { 
		return
	} 
	token := r.FormValue("token")  







	if err := epoller.Add(conn,token); err != nil {
		log.Printf("Failed to add connection %v", err)
		conn.Close()
	} 

}

func main() {
	// Increase resources limitations
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}

	// Enable pprof hooks
	go func() {
		if err := http.ListenAndServe("localhost:6061", nil); err != nil {
			log.Fatalf("pprof failed: %v", err)
		}
	}()

	// Start epoll
	var err error
	epoller, err = MkEpoll()
	if err != nil {
		panic(err)
	}

	go Start()

	http.HandleFunc("/", wsHandler)
	//err := http.ListenAndServeTLS(":8443", "cert.pem", "key.pem", nil)
	//https://godoc.org/net/http#example-ListenAndServeTLS
	//por si no funciona intentar lo que dice garyburd o guybrand en client https://github.com/gorilla/websocket/issues/158
	if err := http.ListenAndServe("0.0.0.0:5001", nil); err != nil {
		log.Fatal(err)
	}
}

func Start() {
	for {
		connections, err := epoller.Wait()
		if err != nil {
			log.Printf("Failed to epoll wait %v", err)
			continue
		}
		for token, conn := range connections {
			if conn == nil {
				break
			}
			if msg, op, err := wsutil.ReadClientData(conn); err != nil {
				if err := epoller.Remove(conn); err != nil {
					log.Printf("Failed to remove %v", err)
				}else{
					log.Printf("removed conn %v",conn ) 
					log.Printf("removed token %v",token ) 
					delete(epoller.token,token) 
				}
				conn.Close()
			} else {
				msgstring := string(msg)
				ss := strings.Split(msgstring, "|")
				if ss[0] != "" {
				   	err = wsutil.WriteServerMessage(epoller.token[ss[0]], op, msg)
					if err != nil {
						// handle error
					} 
				}  
			}
		}
	}
}

type epoll struct {
	fd          int
	connections map[int]net.Conn 
	token		map[string]net.Conn
	lock        *sync.RWMutex
}

func MkEpoll() (*epoll, error) {
	fd, err := unix.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	return &epoll{
		fd:          fd,
		lock:        &sync.RWMutex{},
		connections: make(map[int]net.Conn), 
		token: 		 make(map[string]net.Conn),  
	}, nil
}

func (e *epoll) Add(conn net.Conn, token string ) error {
	// Extract file descriptor associated with the connection
	fd := websocketFD(conn)
	err := unix.EpollCtl(e.fd, syscall.EPOLL_CTL_ADD, fd, &unix.EpollEvent{Events: unix.POLLIN | unix.POLLHUP, Fd: int32(fd)})
	if err != nil {
		return err
	}
	e.lock.Lock()
	defer e.lock.Unlock()
	e.connections[fd] = conn
	e.token[token] = conn
	if len(e.connections)%100 == 0 {
		log.Printf("Total number of connections: %v", len(e.connections))
	}
	return nil
}

func (e *epoll) Remove(conn net.Conn) error {
	fd := websocketFD(conn)
	err := unix.EpollCtl(e.fd, syscall.EPOLL_CTL_DEL, fd, nil)
	if err != nil {
		return err
	}
	e.lock.Lock()
	defer e.lock.Unlock()  
	delete(e.connections, fd) 
	if len(e.connections)%100 == 0 {
		log.Printf("Total number of connections: %v", len(e.connections))
	}
	return nil
}

func (e *epoll) Wait() (map[string]net.Conn, error) {
	events := make([]unix.EpollEvent, 100)
	n, err := unix.EpollWait(e.fd, events, 100)
	if err != nil {
		return nil, err
	}
	e.lock.RLock()
	defer e.lock.RUnlock()
	keys := reflect.ValueOf(e.token).MapKeys() 
	connections := make(map[string]net.Conn)
	for i := 0; i < n; i++ {
 
		connections[keys[i].String()] = e.connections[int(events[i].Fd)]
	} 
	return connections, nil
}

func websocketFD(conn net.Conn) int {
	//tls := reflect.TypeOf(conn.UnderlyingConn()) == reflect.TypeOf(&tls.Conn{})
	// Extract the file descriptor associated with the connection
	//connVal := reflect.Indirect(reflect.ValueOf(conn)).FieldByName("conn").Elem()
	tcpConn := reflect.Indirect(reflect.ValueOf(conn)).FieldByName("conn")
	//if tls {
	//	tcpConn = reflect.Indirect(tcpConn.Elem())
	//}
	fdVal := tcpConn.FieldByName("fd")
	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")

	return int(pfdVal.FieldByName("Sysfd").Int())
}