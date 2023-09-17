package esl

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
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
	ConnectionHandler func(*EslOutboundConnection)
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
		go server.ConnectionHandler(&esl)
	}
}

type EslOutboundConnection struct {
	ESLConnection
	channelData ESLMessage
}

func (esl *EslOutboundConnection) Connect() error {
	esl.reader = bufio.NewReader(esl.socket)
	esl.writer = bufio.NewWriter(esl.socket)
	esl.SetStatus("ready") //this is hack :(
	go esl.ReadMessages()
	msg, err := esl.SendCMD("connect")
	esl.channelData = msg
	return err
}

func NewOutboundESLConnection(socket net.Conn) EslOutboundConnection {
	conn := EslOutboundConnection{
		ESLConnection: NewEslConnection(),
		channelData:   ESLMessage{},
	}
	conn.socket = socket
	return conn
}
