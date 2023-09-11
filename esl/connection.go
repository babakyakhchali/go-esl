package esl

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
)

type ESLConfig struct {
	Host         string
	Port         int
	Password     string
	EnableBgJOBs bool
}

type ESLConnection struct {
	Handlers      map[string]func(ESLMessage)
	EventBindings []string
	Filters       map[string]string
	Config        ESLConfig
	conn          net.Conn
	reader        *bufio.Reader
	writer        *bufio.Writer
	writeMutex    sync.Mutex
	replyChannel  chan ESLMessage
	errorChannel  chan error
	status        string
	jobs          map[string]chan ESLMessage

	Logger      *slog.Logger
	LogMessages bool
}

func (esl *ESLConnection) Init() error {
	err := esl.Connect()
	if err != nil {
		return err
	}

	go esl.ReadMessages()
	_, err = esl.Authenticate()
	if err != nil {
		return err
	}
	err = esl.InitEventBindings()
	if err != nil {
		return err
	}
	return nil
}

func (esl *ESLConnection) Connect() error {
	var err error
	esl.SetStatus("connecting")
	esl.conn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", esl.Config.Host, esl.Config.Port))
	if err != nil {
		return err
	}
	esl.Logger.Debug("connected", "config", esl.Config)
	esl.reader = bufio.NewReader(esl.conn)
	esl.writer = bufio.NewWriter(esl.conn)
	esl.SetStatus("ready")
	return nil
}

func (esl *ESLConnection) ReadLine() (string, error) {
	line, err := esl.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (esl *ESLConnection) Read(n int) (string, error) {
	bytes := make([]byte, n)
	bytesRead, err := io.ReadFull(esl.reader, bytes)
	if err != nil {
		return "", err
	}
	if n != bytesRead {
		return "", fmt.Errorf("insufficient bytes read")
	}
	return string(bytes), nil
}

func (esl *ESLConnection) readMSG() (ESLMessage, error) {
	return ParseESLStream(esl.reader)
}

func (esl *ESLConnection) API(api string, args string) (ESLMessage, error) {
	return esl.SendCMD(fmt.Sprintf("api %s %s", api, args))
}

func (esl *ESLConnection) BgAPI(api string, args string, jobUUID string) (ESLMessage, error) {
	if jobUUID == "" {
		jobUUID = uuid.New().String()
	}

	return esl.SendCMD(fmt.Sprintf("bgapi %s %s\nJob-UUID: %s", api, args, jobUUID))
}

func (esl *ESLConnection) BgAPIWithResult(api string, args string, timeout time.Duration) (ESLMessage, error) {
	jobUUID := uuid.New().String()
	jchan := make(chan ESLMessage, 1)
	esl.jobs[jobUUID] = jchan
	esl.SendCMD(fmt.Sprintf("bgapi %s %s\nJob-UUID: %s", api, args, jobUUID))

	select {
	case nmsg := <-jchan:
		return nmsg, nil
	case <-time.After(timeout * time.Second):
		return ESLMessage{}, fmt.Errorf("job timeout")
	}
}

func (esl *ESLConnection) SendCMD(msg string) (ESLMessage, error) {
	return esl.SendCMDTimed(msg, 0)
}

func (esl *ESLConnection) IsReady() bool {
	return esl.status == "ready"
}

func (esl *ESLConnection) SendRecvTimed(msg string, timeout time.Duration) (ESLMessage, error) {
	esl.Logger.Debug("trying to lock write log", "message", msg)
	esl.writeMutex.Lock()
	defer esl.writeMutex.Unlock()
	esl.Logger.Debug("lock aquired", "message", msg)
	err := esl.Write(msg + "\n\n")
	if err != nil {
		esl.Logger.Error("socket write error", "error", err)
		return ESLMessage{}, err
	}
	if timeout > 0 {
		select {
		case nmsg := <-esl.replyChannel:
			return nmsg, nil
		case <-time.After(timeout * time.Second):
			esl.Logger.Error("socket write error", "error", "timeout")
			return ESLMessage{}, fmt.Errorf("esl timeout")
		}
	} else {
		nmsg := <-esl.replyChannel
		return nmsg, nil
	}
}

func (esl *ESLConnection) SendCMDTimed(msg string, timeout time.Duration) (ESLMessage, error) {
	return esl.SendRecvTimed(msg+"\n\n", timeout)
}

func (esl *ESLConnection) SendCMDf(msg string, a ...any) (ESLMessage, error) {
	cmd := fmt.Sprintf(msg, a...)
	return esl.SendCMD(cmd)
}

func (esl *ESLConnection) Writef(msg string, a ...any) error {
	_, err := fmt.Fprintf(esl.conn, msg, a...)
	if err != nil {
		return err
	}
	return nil
}

func (esl *ESLConnection) Write(msg string) error {
	if !esl.IsReady() {
		return fmt.Errorf("esl not ready")
	}
	_, err := fmt.Fprint(esl.conn, msg)
	if err != nil {
		return err
	}
	return nil
}

func (esl *ESLConnection) Authenticate() (ESLMessage, error) {
	msg := <-esl.replyChannel

	if msg.ContentType == "auth/request" {
		msg, err := esl.SendCMD("auth " + esl.Config.Password)
		if err != nil {
			return msg, err
		}
		if msg.GetReply() != "+OK accepted" {
			return msg, fmt.Errorf("authentication Error")
		}

	}
	return msg, nil
}

func (esl *ESLConnection) AddFilter(eventHeader string, headerValue string) (ESLMessage, error) {
	event, err := esl.SendCMDf("filter %s %s", eventHeader, headerValue)
	if err != nil {
		return event, err
	}
	if reply := event.GetReply(); !strings.HasPrefix(reply, "+OK") {
		return event, fmt.Errorf("adding filter [%s] [%s] failed, esl reply:%s", eventHeader, headerValue, reply)
	}
	return event, err
}

func (esl *ESLConnection) InitFilters() error {
	for k, v := range esl.Filters {
		event, err := esl.SendCMDf("filter %s %s", k, v)
		if err != nil {
			return err
		}
		if reply := event.GetReply(); !strings.HasPrefix(reply, "+OK") {
			return fmt.Errorf("adding filter [%s] [%s] failed, esl reply:%s", k, v, reply)
		}
	}
	return nil
}

func (esl *ESLConnection) InitEventBindings() error {
	if esl.Config.EnableBgJOBs && !slices.Contains(esl.EventBindings, "BACKGROUND_JOB") {
		esl.EventBindings = append(esl.EventBindings, "BACKGROUND_JOB")
	}
	if len(esl.EventBindings) == 0 {
		return nil
	}
	events := strings.Join(esl.EventBindings, " ")
	cmd := fmt.Sprintf("events plain %s", events)
	if events == "*" {
		cmd = "events plain all"
	}
	event, err := esl.SendCMDf(cmd)
	if err != nil {
		return err
	}
	if reply := event.GetReply(); !strings.HasPrefix(reply, "+OK") {
		return fmt.Errorf("adding event binding for [%s] failed, esl reply:%s", events, reply)
	}
	return err
}

func (esl *ESLConnection) SetStatus(s string) {
	esl.Logger.Debug("change status from " + esl.status + " to " + s)
	esl.status = s
}

func (esl *ESLConnection) ReadMessages() {
	defer esl.SetStatus("stopped")
	for {
		l := esl.Logger.With("func", "ReadMessages")
		msg, err := esl.readMSG() //EOF is returned if socket is closed
		if err != nil {
			l.Debug("end with error", "error", err)
			esl.errorChannel <- err
			return
		}
		l.Debug("got msg", "msg", msg)
		if msg.ContentType == "auth/request" {
			esl.replyChannel <- msg
		} else if msg.ContentType == "text/disconnect-notice" {
			esl.errorChannel <- fmt.Errorf("disconnected with cause %s", msg.Body)
			l.Debug("got text/disconnect-notice")
			return
		} else if msg.ContentType == "command/reply" {
			esl.replyChannel <- msg
		} else if msg.ContentType == "api/response" {
			esl.replyChannel <- msg
		} else if msg.ContentType == "text/event-plain" {
			if msg.ContentLength > 0 {
				MergeEventBody(&msg)
			}
			eventName := msg.GetEventName()
			if eventName == "BACKGROUND_JOB" || eventName == "CHANNEL_EXECUTE_COMPLETE" {
				key := "Application-UUID"
				if eventName == "BACKGROUND_JOB" {
					key = "Job-UUID"
				}
				jobUUID := msg.Headers[key]
				if jchan, exists := esl.jobs[jobUUID]; exists {
					jchan <- msg
				}
				delete(esl.jobs, jobUUID)
			}
			if handler, exists := esl.Handlers[eventName]; exists {
				handler(msg)
			} else if handler, exists := esl.Handlers["*"]; exists {
				handler(msg)
			}
		}
	}
}

func (esl *ESLConnection) Wait() error {
	err := <-esl.errorChannel
	return err
}

func (esl *ESLConnection) CLose() error {
	esl.writeMutex.Lock()
	esl.SetStatus("closed")
	defer esl.writeMutex.Unlock()
	return esl.conn.Close()
}

func NewInboundESLConnection(config ESLConfig) ESLConnection {
	return ESLConnection{
		Handlers:      map[string]func(ESLMessage){},
		EventBindings: []string{},
		Filters:       map[string]string{},
		Config:        config,
		conn:          nil,
		reader:        &bufio.Reader{},
		writer:        &bufio.Writer{},
		writeMutex:    sync.Mutex{},
		replyChannel:  make(chan ESLMessage),
		errorChannel:  make(chan error, 1),
		jobs:          map[string]chan ESLMessage{},
		LogMessages:   false,
		Logger:        slog.New(slog.NewTextHandler(os.Stdout, nil)),
		status:        "created",
	}
}
