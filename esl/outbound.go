package esl

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
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

type AsyncEslAction struct {
	ErrorChannel  chan error
	ResultChannel chan ESLMessage
	SendResult    ESLMessage
	timeout       time.Duration
}

func (action *AsyncEslAction) Wait() (ESLMessage, error) {
	if action.timeout > 0 {
		select {
		case msg := <-action.ResultChannel:
			return msg, nil
		case err := <-action.ErrorChannel:
			return ESLMessage{}, err
		case <-time.After(action.timeout):
			return ESLMessage{}, fmt.Errorf("execution timeout")
		}
	} else {
		select {
		case msg := <-action.ResultChannel:
			return msg, nil
		case err := <-action.ErrorChannel:
			return ESLMessage{}, err
		}
	}

}

type ExecutionOptions struct {
	App         string
	Args        string
	ChannelUUID string
	Lock        bool
	Loops       int
	AppUUID     string
	Timeout     time.Duration
}

func (esl *ESLConnection) Execute(options ExecutionOptions) (AsyncEslAction, error) {
	msg := ESLMessage{
		msgString:     "",
		ContentType:   "",
		ContentLength: 0,
		Body:          "",
		Headers:       map[string]string{},
	}

	if options.Lock {
		msg.Headers["event-lock"] = "true"
	}

	msg.Headers["call-command"] = "execute"
	msg.Headers["execute-app-name"] = options.App
	if options.Args != "" {
		msg.Headers["execute-app-arg"] = options.Args
	}
	if options.Loops > 0 {
		msg.Headers["loops"] = strconv.Itoa(options.Loops)
	}
	if options.AppUUID == "" {
		options.AppUUID = uuid.NewString()
	}

	result := AsyncEslAction{
		ErrorChannel:  make(chan error, 1),
		ResultChannel: make(chan ESLMessage, 1),
		SendResult:    ESLMessage{},
	}

	msg.Headers["Event-UUID"] = options.AppUUID
	esl.jobs[options.AppUUID] = result

	r, err := esl.SendMSG(msg, options.ChannelUUID)
	result.SendResult = r
	return result, err
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
		errorChannel:  make(chan error, 1),
		jobs:          map[string]AsyncEslAction{},
		LogMessages:   false,
		Logger:        slog.New(slog.NewTextHandler(os.Stdout, nil)),
		status:        "created",
		replyTimeout:  0,
	}
}
