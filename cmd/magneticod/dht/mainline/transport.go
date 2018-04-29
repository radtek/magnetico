package mainline

import (
	"net"

	"github.com/anacrolix/torrent/bencode"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

type Transport struct {
	fd      int
	laddr   *net.UDPAddr
	started bool

	// OnMessage is the function that will be called when Transport receives a packet that is
	// successfully unmarshalled as a syntactically correct Message (but -of course- the checking
	// the semantic correctness of the Message is left to Protocol).
	onMessage func(*Message, net.Addr)
}

func NewTransport(laddr string, onMessage func(*Message, net.Addr)) *Transport {
	transport := new(Transport)
	transport.onMessage = onMessage
	var err error
	transport.laddr, err = net.ResolveUDPAddr("udp", laddr)
	if err != nil {
		zap.L().Panic("Could not resolve the UDP address for the trawler!", zap.Error(err))
	}

	return transport
}

func (t *Transport) Start() {
	// Why check whether the Transport `t` started or not, here and not -for instance- in
	// t.Terminate()?
	// Because in t.Terminate() the programmer (i.e. you & me) would stumble upon an error while
	// trying close an uninitialised net.UDPConn or something like that: it's mostly harmless
	// because its effects are immediate. But if you try to start a Transport `t` for the second
	// (or the third, 4th, ...) time, it will keep spawning goroutines and any small mistake may
	// end up in a debugging horror.
	//                                                                   Here ends my justification.
	if t.started {
		zap.L().Panic("Attempting to Start() a mainline/Transport that has been already started! (Programmer error.)")
	}
	t.started = true

	var err error
	t.fd, err = unix.Socket(unix.SOCK_DGRAM, unix.AF_INET, 0)
	if err != nil {
		zap.L().Fatal("Could NOT create a UDP socket!", zap.Error(err))
	}

	unix.Bind(t.fd, unix.SockaddrInet4{Addr: t.laddr.IP, Port: t.laddr.Port})

	go t.readMessages()
}

func (t *Transport) Terminate() {
	unix.Close(t.fd);
}

// readMessages is a goroutine!
func (t *Transport) readMessages() {
	buffer := make([]byte, 65536)

	for {
		n, from, err := unix.Recvfrom(t.fd, buffer, 0)
		if err != nil {
			// TODO: isn't there a more reliable way to detect if UDPConn is closed?
			zap.L().Debug("Could NOT read an UDP packet!", zap.Error(err))
		}

		var msg Message
		err = bencode.Unmarshal(buffer[:n], &msg)
		if err != nil {
			zap.L().Debug("Could NOT unmarshal packet data!", zap.Error(err))
		}

		t.onMessage(&msg, from)
	}
}

func (t *Transport) WriteMessages(msg *Message, addr net.Addr) {
	data, err := bencode.Marshal(msg)
	if err != nil {
		zap.L().Panic("Could NOT marshal an outgoing message! (Programmer error.)")
	}

	err = unix.Sendto(t.fd, data, 0, addr)
	// TODO: isn't there a more reliable way to detect if UDPConn is closed?
	if err != nil {
		zap.L().Debug("Could NOT write an UDP packet!", zap.Error(err))
	}
}