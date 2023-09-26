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
	"golang.org/x/exp/slog"
)

type ESLConfig struct {
	Host         string
	Port         int
	Password     string
	EnableBgJOBs bool
}

type ESLConnection struct {
	handlers     map[string]func(ESLMessage)
	enableAsync  bool
	Filters      map[string]string
	socket       net.Conn
	reader       *bufio.Reader
	writer       *bufio.Writer
	writeMutex   sync.Mutex
	replyChannel chan ESLMessage
	errorChannel chan error
	status       string
	jobs         map[string]AsyncEslAction

	logger            *slog.Logger
	logMessages       bool
	logMessagesFormat string //full|brief empty is full
	replyTimeout      time.Duration
}

func (esl *ESLConnection) SetLogger(logger *slog.Logger) {
	esl.logger = logger
}

func (esl *ESLConnection) EnableAsync() (ESLMessage, error) {
	esl.enableAsync = true
	cmd := fmt.Sprintf("events plain %s %s ", EventBackgroundJob, EventChannelExecuteComplete)
	return esl.SendCMDf(cmd)
}

func (esl *ESLConnection) EnableMessageLogging(format string) {
	esl.logMessages = true
	esl.logMessagesFormat = format
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

func (esl *ESLConnection) BgAPIWithResult(api string, args string, jobUUID string, timeout time.Duration) (AsyncEslAction, error) {
	result := AsyncEslAction{
		ErrorChannel:  make(chan error, 1),
		ResultChannel: make(chan ESLMessage, 1),
		SendResult:    ESLMessage{},
		timeout:       timeout,
	}
	if jobUUID == "" {
		jobUUID = uuid.New().String()
	}

	esl.jobs[jobUUID] = result
	msg, err := esl.SendCMD(fmt.Sprintf("bgapi %s %s\nJob-UUID: %s", api, args, jobUUID))
	result.SendResult = msg
	return result, err
}

func (esl *ESLConnection) SendCMD(msg string) (ESLMessage, error) {
	return esl.SendCMDTimed(msg, 0)
}

func (esl *ESLConnection) IsReady() bool {
	return esl.status == "ready"
}

func (esl *ESLConnection) SendRecvTimed(msg string, timeout time.Duration) (ESLMessage, error) {
	esl.logger.Debug("trying to lock write log", "message", msg)
	esl.writeMutex.Lock()
	defer esl.writeMutex.Unlock()
	esl.logger.Debug("lock aquired", "message", msg)
	err := esl.Write(msg + "\n\n")
	if err != nil {
		esl.logger.Error("socket write error", "error", err)
		return ESLMessage{}, err
	}
	if timeout > 0 {
		select {
		case nmsg := <-esl.replyChannel:
			return nmsg, nil
		case <-time.After(timeout * time.Second):
			esl.logger.Error("socket write error", "error", "timeout")
			return ESLMessage{}, fmt.Errorf("esl timeout")
		}
	} else {
		nmsg := <-esl.replyChannel
		return nmsg, nil
	}
}

func (esl *ESLConnection) SendCMDTimed(msg string, timeout time.Duration) (ESLMessage, error) {
	return esl.SendRecvTimed(msg, timeout)
}

func (esl *ESLConnection) SendCMDf(msg string, a ...any) (ESLMessage, error) {
	cmd := fmt.Sprintf(msg, a...)
	return esl.SendCMD(cmd)
}

func (esl *ESLConnection) Writef(msg string, a ...any) error {
	_, err := fmt.Fprintf(esl.socket, msg, a...)
	if err != nil {
		return err
	}
	return nil
}

func (esl *ESLConnection) Write(msg string) error {
	if !esl.IsReady() {
		return fmt.Errorf("esl not ready")
	}
	_, err := fmt.Fprint(esl.socket, msg)
	if err != nil {
		return err
	}
	return nil
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

func (esl *ESLConnection) DeleteFilter(eventHeader string, headerValue string) (ESLMessage, error) {
	event, err := esl.SendCMDf("filter delete %s %s", eventHeader, headerValue)
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

func (esl *ESLConnection) ApplyEventBindings() (ESLMessage, error) {
	cmd := "events plain "
	if _, exists := esl.handlers["*"]; exists {
		cmd += "all"
	} else {
		for eventName := range esl.handlers {
			cmd += " " + eventName
		}
		if esl.enableAsync {
			cmd += fmt.Sprintf(" %s %s ", EventBackgroundJob, EventChannelExecuteComplete)
		}
	}

	return esl.SendCMDf(cmd)
}

func (esl *ESLConnection) AddEventBinding(eventName string, handler func(ESLMessage)) (msg ESLMessage, err error) {
	if _, exists := esl.handlers[eventName]; exists {
		return
	}
	esl.handlers[eventName] = handler
	msg, err = esl.SendCMDf("events plain " + eventName)
	if err != nil {
		return
	}
	return
}

func (esl *ESLConnection) AddGlobalEventHandler(handler func(ESLMessage)) (ESLMessage, error) {
	esl.handlers["*"] = handler
	return esl.SendCMDf("events plain all")
}

func (esl *ESLConnection) AddEventBindings(bindings map[string]func(ESLMessage)) (ESLMessage, error) {
	for eventName, handler := range bindings {
		esl.handlers[eventName] = handler
	}
	return esl.ApplyEventBindings()
}

func (esl *ESLConnection) EnableAsyncSupport() {
	esl.enableAsync = true
}

func (esl *ESLConnection) SetStatus(s string) {
	esl.logger.Debug("change status from " + esl.status + " to " + s)
	esl.status = s
}

func (esl *ESLConnection) notifyJobsForError(err error) {
	for _, result := range esl.jobs {
		result.ErrorChannel <- err
	}
}

// ReadMessages read esl messages from socket, if handlers are registered for events they will be run in
// new goroutines
func (esl *ESLConnection) ReadMessages() {
	defer esl.SetStatus("stopped")
	for {
		l := esl.logger.With("func", "ReadMessages")
		l.Debug("waiting for esl msg")
		msg, err := esl.readMSG() //EOF is returned if socket is closed
		if err != nil {
			l.Debug("end with error", "error", err)
			esl.errorChannel <- err
			esl.notifyJobsForError(err)
			return
		}

		if msg.ContentType == ContentTypeAuthRequest {
			esl.replyChannel <- msg
		} else if msg.ContentType == ContentTypeTextDisconnectNotice {
			esl.errorChannel <- fmt.Errorf("disconnected with cause %s", msg.Body)
			esl.notifyJobsForError(err)
			l.Debug("got text/disconnect-notice")
			return
		} else if msg.ContentType == ContentTypeCommandReply {
			esl.replyChannel <- msg
		} else if msg.ContentType == ContentTypeApiResponse {
			esl.replyChannel <- msg
		} else if msg.ContentType == ContentTypeTextEventPlain {
			if msg.ContentLength > 0 {
				MergeEventBody(&msg)
			}
			eventName := msg.GetEventName()
			if esl.enableAsync && (eventName == EventBackgroundJob || eventName == EventChannelExecuteComplete) {
				key := MessageHeaderApplicationUUID
				if eventName == EventBackgroundJob {
					key = MessageHeaderJobUuid
				}
				jobUUID := msg.Headers[key]
				if asyncResult, exists := esl.jobs[jobUUID]; exists {
					asyncResult.ResultChannel <- msg
				}
				delete(esl.jobs, jobUUID)
			}
			if handler, exists := esl.handlers[eventName]; exists {
				go handler(msg)
			} else if handler, exists := esl.handlers["*"]; exists {
				go handler(msg)
			}
		}
		if esl.logMessages {
			if esl.logMessagesFormat == "brief" {
				l.Debug("new esl msg", "msg", msg.StringBrief())
			} else {
				l.Debug("new esl msg", "msg", msg.String())
			}

		}
	}
}

func (esl *ESLConnection) Wait() error {
	err := <-esl.errorChannel
	return err
}

func (esl *ESLConnection) MyEvents(handlers map[string]func(ESLMessage)) (msg ESLMessage, err error) {
	esl.handlers = handlers
	return esl.SendCMD("myevents")
}

func (esl *ESLConnection) CLose() error {
	esl.writeMutex.Lock()
	esl.SetStatus("closed")
	defer esl.writeMutex.Unlock()
	return esl.socket.Close()
}

func NewEslConnection() *ESLConnection {
	return &ESLConnection{
		handlers:     map[string]func(ESLMessage){},
		Filters:      map[string]string{},
		socket:       nil,
		reader:       &bufio.Reader{},
		writer:       &bufio.Writer{},
		writeMutex:   sync.Mutex{},
		replyChannel: make(chan ESLMessage),
		errorChannel: make(chan error, 1),
		jobs:         map[string]AsyncEslAction{},
		logMessages:  false,
		logger:       slog.New(slog.NewTextHandler(os.Stdout, nil)),
		status:       "created",
		replyTimeout: 0,
	}
}
