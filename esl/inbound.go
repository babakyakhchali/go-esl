package esl

import (
	"bufio"
	"fmt"
	"net"
)

type EslInboundConnection struct {
	ESLConnection
	config ESLConfig
}

func (esl *EslInboundConnection) Authenticate() (ESLMessage, error) {
	msg := <-esl.replyChannel

	if msg.ContentType == "auth/request" {
		msg, err := esl.SendCMD("auth " + esl.config.Password)
		if err != nil {
			return msg, err
		}
		if msg.GetReply() != "+OK accepted" {
			return msg, fmt.Errorf("authentication Error")
		}

	}
	return msg, nil
}

func (esl *EslInboundConnection) Connect() error {
	var err error
	esl.SetStatus("connecting")
	esl.socket, err = net.Dial("tcp", fmt.Sprintf("%s:%d", esl.config.Host, esl.config.Port))
	if err != nil {
		return err
	}
	esl.Logger.Debug("connected", "config", esl.config)
	esl.reader = bufio.NewReader(esl.socket)
	esl.writer = bufio.NewWriter(esl.socket)
	esl.SetStatus("connected")
	return nil
}

func NewInboundESLConnection(config ESLConfig) EslInboundConnection {
	return EslInboundConnection{
		ESLConnection: NewEslConnection(),
		config:        config,
	}
}
