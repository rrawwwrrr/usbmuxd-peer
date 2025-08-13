package ports

import (
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type PortForwarder struct {
	mu          sync.Mutex
	activePorts map[string]bool // {"3333": true, "4444": false}
}

func (pf *PortForwarder) isPortOpen(port string) bool {
	conn, err := net.DialTimeout("tcp", "localhost:"+port, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (pf *PortForwarder) monitorPorts() {
	for {
		pf.mu.Lock()
		for port := range pf.activePorts {
			pf.activePorts[port] = pf.isPortOpen(port)
		}
		pf.mu.Unlock()
		time.Sleep(2 * time.Second) // Проверяем каждые 2 секунды
	}
}

func (pf *PortForwarder) handleClient(conn net.Conn) {
	defer conn.Close()

	pf.mu.Lock()
	defer pf.mu.Unlock()

	for port, isActive := range pf.activePorts {
		if !isActive {
			continue
		}

		targetConn, err := net.Dial("tcp", "localhost:"+port)
		if err != nil {
			log.Printf("Failed to connect to port %s: %v", port, err)
			continue
		}
		defer targetConn.Close()

		go io.Copy(targetConn, conn)
		io.Copy(conn, targetConn)
		return
	}

	log.Println("No active ports available")
}

func StartPortForwarder() (*PortForwarder, error) {
	pf := &PortForwarder{
		activePorts: map[string]bool{
			"3333": false,
			"4444": false,
		},
	}

	go pf.monitorPorts()

	listener, err := net.Listen("tcp", ":5555")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	log.Println("Server started on :5555, monitoring ports 3333 and 4444...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		go pf.handleClient(conn)
	}
}
