package server

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A data socket is used to send non-control data between the client and
// server.
type DataSocket interface {
	Host() string

	Port() int

	// the standard io.Reader interface
	Read(p []byte) (n int, err error)

	// the standard io.Writer interface
	Write(p []byte) (n int, err error)

	// the standard io.Closer interface
	Close() error
}

type ftpActiveSocket struct {
	conn   net.Conn
	host   string
	port   int
	logger Logger
}

func newActiveSocket(remote string, port int, logger Logger, sessionID string) (DataSocket, error) {
	connectTo := net.JoinHostPort(remote, strconv.Itoa(port))

	logger.Print(sessionID, "Opening active data connection to "+connectTo)

	raddr, err := net.ResolveTCPAddr("tcp", connectTo)

	if err != nil {
		logger.Print(sessionID, err)
		return nil, err
	}

	tcpConn, err := net.DialTimeout("tcp", raddr.String(), 15*time.Second)

	if err != nil {
		logger.Print(sessionID, err)
		return nil, err
	}

	socket := new(ftpActiveSocket)
	socket.conn = tcpConn
	socket.host = remote
	socket.port = port
	socket.logger = logger

	fmt.Println("open", socket.port)

	return socket, nil
}

func (socket *ftpActiveSocket) Host() string {
	return socket.host
}

func (socket *ftpActiveSocket) Port() int {
	return socket.port
}

func (socket *ftpActiveSocket) Read(p []byte) (n int, err error) {
	return socket.conn.Read(p)
}

func (socket *ftpActiveSocket) Write(p []byte) (n int, err error) {
	return socket.conn.Write(p)
}

func (socket *ftpActiveSocket) Close() error {
	fmt.Println("close", socket.port)
	return socket.conn.Close()
}

type ftpPassiveSocket struct {
	conn      net.Conn
	port      int
	host      string
	ingress   chan []byte
	egress    chan []byte
	logger    Logger
	wg        sync.WaitGroup
	err       error
	tlsConfig *tls.Config
}

func newPassiveSocket(host string, port int, logger Logger, sessionID string, tlsConfing *tls.Config) (DataSocket, error) {
	socket := new(ftpPassiveSocket)
	socket.ingress = make(chan []byte)
	socket.egress = make(chan []byte)
	socket.logger = logger
	socket.host = host
	socket.port = port
	if err := socket.GoListenAndServe(sessionID); err != nil {
		return nil, err
	}

	fmt.Println("open", socket.port)

	return socket, nil
}

func (socket *ftpPassiveSocket) Host() string {
	return socket.host
}

func (socket *ftpPassiveSocket) Port() int {
	return socket.port
}

func (socket *ftpPassiveSocket) Read(p []byte) (int, error) {
	err := socket.waitForOpenSocket()

	if err != nil {
		return 0, err
	}

	return socket.conn.Read(p)
}

func (socket *ftpPassiveSocket) Write(p []byte) (int, error) {
	err := socket.waitForOpenSocket()

	if err != nil {
		return 0, err
	}

	return socket.conn.Write(p)
}

func (socket *ftpPassiveSocket) Close() error {
	if socket.conn != nil {
		fmt.Println("close", socket.port)
		return socket.conn.Close()
	}

	return nil
}

func (socket *ftpPassiveSocket) GoListenAndServe(sessionID string) (err error) {
	laddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", strconv.Itoa(socket.port)))

	if err != nil {
		socket.logger.Print(sessionID, err)
		return
	}

	var listener net.Listener

	listener, err = net.ListenTCP("tcp", laddr)

	if err != nil {
		socket.logger.Print(sessionID, err)
		return
	}

	add := listener.Addr()
	parts := strings.Split(add.String(), ":")
	port, err := strconv.Atoi(parts[len(parts)-1])

	if err != nil {
		socket.logger.Print(sessionID, err)
		return
	}

	socket.port = port
	socket.wg.Add(1)

	if socket.tlsConfig != nil {
		listener = tls.NewListener(listener, socket.tlsConfig)
	}

	go func() {
		listener.(*net.TCPListener).SetDeadline(time.Now().Add(15 * time.Second))

		conn, err := listener.Accept()

		defer socket.wg.Done()

		if err != nil {
			socket.err = err
			return
		}

		socket.err = nil
		socket.conn = conn
	}()

	return nil
}

func (socket *ftpPassiveSocket) waitForOpenSocket() error {
	if socket.conn != nil {
		return nil
	}

	socket.wg.Wait()

	return socket.err
}
