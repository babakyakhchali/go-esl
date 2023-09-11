package esl

import (
	"bufio"
	"net"
	"os"
	"strconv"
	"sync"

	"golang.org/x/exp/slog"
)

func (esl *ESLConnection) SendMSG(msg ESLMessage, uuid string) (ESLMessage, error) {
	s := "sendmsg"
	if uuid != "" {
		s += " " + uuid
	}
	s += "\n"

	for hdr, val := range msg.Headers {
		s += hdr + ": " + val + "\n"
	}

	if msg.Body != "" {
		s += "Content-Length: " + strconv.Itoa(len(msg.Body)) + "\n\n" + msg.Body
	}

	return esl.SendRecvTimed(s, 0)
}

func (esl *ESLConnection) Execute(app string, args string, uuid string, lock bool, loops int, appUUID string) (ESLMessage, error) {
	msg := ESLMessage{
		msgString:     "",
		ContentType:   "",
		ContentLength: 0,
		Body:          "",
		Headers:       map[string]string{},
	}

	if lock {
		msg.Headers["event-lock"] = "true"
	}

	msg.Headers["call-command"] = "execute"
	msg.Headers["execute-app-name"] = app
	if args != "" {
		msg.Headers["execute-app-arg"] = args
	}
	if loops > 0 {
		msg.Headers["loops"] = strconv.Itoa(loops)
	}
	if appUUID != "" {
		msg.Headers["Event-UUID"] = appUUID
	}
	return esl.SendMSG(msg, uuid)
}

type ESLServer struct {
	Address           string
	ConnectionHandler func(*ESLConnection)
	Logger            *slog.Logger
	LogMessages       bool
}

func (server *ESLServer) Run() error {
	listen, err := net.Listen("tcp", server.Address)
	if err != nil {
		return err
	}
	defer listen.Close()
	server.Logger.Info("server started on " + server.Address)
	for {
		conn, err := listen.Accept()
		server.Logger.Debug("new connection from " + conn.RemoteAddr().String())
		if err != nil {
			return err
		}
		esl := NewOutboundESLConnection(conn)
		esl.LogMessages = server.LogMessages
		esl.Logger = server.Logger
		esl.SetStatus("ready")
		go esl.ReadMessages()
		go server.ConnectionHandler(&esl)
	}
}

func NewOutboundESLConnection(conn net.Conn) ESLConnection {
	return ESLConnection{
		Handlers:      map[string]func(ESLMessage){},
		EventBindings: []string{},
		Filters:       map[string]string{},
		Config:        ESLConfig{},
		conn:          conn,
		reader:        bufio.NewReader(conn),
		writer:        bufio.NewWriter(conn),
		writeMutex:    sync.Mutex{},
		replyChannel:  make(chan ESLMessage),
		errorChannel:  make(chan error),
		jobs:          map[string]chan ESLMessage{},
		LogMessages:   false,
		Logger:        slog.New(slog.NewTextHandler(os.Stdout, nil)),
		status:        "created",
	}
}
