package socket

import (
	"io"
	"net"
	"os"

	log "github.com/sirupsen/logrus"
)

const (
	tcpListenAddr = ":27015"
	unixSocket    = "/var/run/usbmuxd"
)

func handleConnection(tcpConn net.Conn) {
	defer tcpConn.Close()

	unixConn, err := net.Dial("unix", unixSocket)
	if err != nil {
		log.Printf("Ошибка подключения к unix socket: %v", err)
		return
	}
	defer unixConn.Close()

	// Два горутина для двунаправленной передачи
	go func() {
		_, err := io.Copy(unixConn, tcpConn)
		if err != nil {
			log.Printf("Ошибка записи в unix socket: %v", err)
		}
		unixConn.(*net.UnixConn).CloseWrite()
	}()

	_, err = io.Copy(tcpConn, unixConn)
	if err != nil {
		log.Printf("Ошибка чтения из unix socket: %v", err)
	}
}

func Start() {
	// Проверка существования сокета
	if _, err := os.Stat(unixSocket); os.IsNotExist(err) {
		log.Fatalf("Unix socket не найден: %s", unixSocket)
	}

	listener, err := net.Listen("tcp", tcpListenAddr)
	if err != nil {
		log.Fatalf("Не удалось запустить TCP-сервер: %v", err)
	}
	defer listener.Close()

	log.Printf("Проксирование %s на TCP %s", unixSocket, tcpListenAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Ошибка подключения: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}
