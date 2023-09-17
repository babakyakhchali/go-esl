package esl

import (
	"fmt"
	"strings"
)

type IEslConnection interface {
	Execute(ExecutionOptions) (AsyncEslAction, error)
	SendCMD(string) (ESLMessage, error)
	IsReady() bool
	MyEvents(handlers map[string]func(ESLMessage)) (msg ESLMessage, err error)
}

type EslSession struct {
	Conn        IEslConnection
	channelData ESLMessage
	ChannelUUID string
}

func (session *EslSession) Execute(app string, args string) (msg ESLMessage, err error) {

	asyncResult, err := session.Conn.Execute(ExecutionOptions{
		App:         app,
		Args:        args,
		ChannelUUID: session.ChannelUUID,
		Lock:        false,
		Loops:       0,
		AppUUID:     "",
		Timeout:     0,
	})
	if err != nil {
		return
	}
	if strings.HasPrefix(asyncResult.SendResult.GetReply(), "-ERR") {
		return ESLMessage{}, fmt.Errorf(asyncResult.SendResult.GetReply())
	}
	//TODO: check sendresult  ,err = -ERR invalid session id
	msg, err = asyncResult.Wait()
	return
}

func (session *EslSession) Answer() (msg ESLMessage, err error) {
	return session.Execute("answer", "")
}

func (session *EslSession) Ready() bool {
	return session.Conn.IsReady()
}

func (session *EslSession) Set(name string, value string) (msg ESLMessage, err error) {
	return session.Execute("set", name+"="+value)
}

func (session *EslSession) GetChannelData() ESLMessage {
	return session.channelData
}
