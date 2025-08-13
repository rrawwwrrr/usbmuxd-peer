package socket

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("component", "server")

var destinations = map[string]string{
	"usbmuxd": "UNIX:/var/run/usbmuxd",
	"forward": "TCP:127.0.0.1:7777",
}

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	opErr, ok := err.(*net.OpError)
	return ok && (opErr.Err.Error() == "use of closed network connection" || opErr.Err.Error() == "connection reset by peer")
}

func isConnectionOpen(conn net.Conn) (bool, error) {
	if conn == nil {
		return false, errors.New("соединение равно nil")
	}
	if _, err := conn.Write(nil); err != nil {
		return false, err
	}
	return true, nil
}

func startProxy(a, b net.Conn) {
	log.WithFields(logrus.Fields{
		"from": a.RemoteAddr(),
		"to":   b.RemoteAddr(),
	}).Info("Начало проксирования")

	var wg sync.WaitGroup
	wg.Add(2)

	closeOnce := func() {
		a.Close()
		b.Close()
	}

	go func() {
		defer wg.Done()
		if ok, _ := isConnectionOpen(a); !ok {
			log.Debug("A уже закрыто, не запускаем A->B")
			return
		}
		_, err := io.Copy(b, a)
		if err != nil && !isClosedError(err) {
			log.WithError(err).WithFields(logrus.Fields{
				"source": a.RemoteAddr(),
				"dest":   b.RemoteAddr(),
			}).Error("Ошибка A->B")
		}
		closeOnce()
	}()

	go func() {
		defer wg.Done()
		if ok, _ := isConnectionOpen(b); !ok {
			log.Debug("B уже закрыто, не запускаем B->A")
			return
		}
		_, err := io.Copy(a, b)
		if err != nil && !isClosedError(err) {
			log.WithError(err).WithFields(logrus.Fields{
				"source": b.RemoteAddr(),
				"dest":   a.RemoteAddr(),
			}).Error("Ошибка B->A")
		}
		closeOnce()
	}()

	wg.Wait()
	log.Info("Проксирование завершено")
}

func handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	clientIP := clientConn.RemoteAddr().String()
	log.WithField("client", clientIP).Debug("Новое подключение")

	reader := bufio.NewReader(clientConn)
	keyLine, err := reader.ReadString('\n')
	if err != nil {
		log.WithError(err).Error("Ошибка чтения handshake")
		return
	}

	key := strings.TrimSpace(keyLine)
	dst, ok := destinations[key]
	if !ok {
		log.WithField("key", key).Warn("Неизвестный ключ маршрута")
		clientConn.Write([]byte("Unknown destination key\n"))
		return
	}

	parts := strings.SplitN(dst, ":", 2)
	if len(parts) != 2 {
		log.Warn("Некорректный формат назначения")
		clientConn.Write([]byte("Invalid destination format\n"))
		return
	}

	var remoteConn net.Conn
	switch parts[0] {
	case "UNIX":
		remoteConn, err = net.Dial("unix", parts[1])
	case "TCP":
		remoteConn, err = net.Dial("tcp", parts[1])
	default:
		log.WithField("proto", parts[0]).Warn("Неизвестный протокол")
		clientConn.Write([]byte("Unknown protocol\n"))
		return
	}

	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"proto": parts[0],
			"addr":  parts[1],
		}).Error("Ошибка подключения к целевому сокету")
		clientConn.Write([]byte("Failed to connect to destination\n"))
		return
	}
	defer remoteConn.Close()

	// Передаем остатки после handshake
	go func() {
		_, err := io.Copy(remoteConn, reader)
		if err != nil && !isClosedError(err) {
			log.WithError(err).Error("Ошибка передачи данных после handshake")
		}
	}()

	startProxy(clientConn, remoteConn)
}

func StartServer() {
	listener, err := net.Listen("tcp", ":27015")
	if err != nil {
		log.WithError(err).Fatal("Ошибка запуска сервера")
	}
	defer listener.Close()
	log.Info("TCP-сервер слушает на :27015")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.WithError(err).Error("Ошибка принятия соединения")
			continue
		}
		go handleConnection(conn)
	}
}
