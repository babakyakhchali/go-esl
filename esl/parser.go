package esl

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

func ParseESLStream(reader *bufio.Reader) (ESLMessage, error) {
	//reader := bufio.NewReader(strings.NewReader(s))

	msg := ESLMessage{
		msgString:     "",
		ContentType:   "",
		ContentLength: 0,
		Body:          "",
		Headers:       map[string]string{},
	}

	for {
		var header, value string
		line, err := reader.ReadString('\n')

		if err != nil {
			return msg, err
		}
		msg.msgString += line

		line = strings.TrimSpace(line)

		if idx := strings.Index(line, ":"); idx > 0 {
			header = line[:idx]
			value = line[idx+2:]
			msg.Headers[header] = value
		}
		if header == "Content-Type" {
			msg.ContentType = value
		}
		if header == "Content-Length" {
			msg.ContentLength, _ = strconv.ParseUint(value, 10, 64)
		}
		if line == "" {
			if msg.ContentLength > 0 {
				bytes := make([]byte, msg.ContentLength)
				readBytesCount, err := io.ReadFull(reader, bytes)
				if msg.ContentLength != uint64(readBytesCount) {
					return msg, fmt.Errorf("not enough bytes read, wanted %d bytes but got %d bytes, buffer:%s", msg.ContentLength, readBytesCount, string(bytes))
				}
				if err != nil {
					return msg, err
				}
				msg.Body = string(bytes)
			}
			return msg, nil
		}
	}
}

func MergeEventBody(msg *ESLMessage) error {
	bodyMSG, err := ParseESLStream(bufio.NewReader(strings.NewReader(msg.Body)))
	if err != nil {
		return err
	}
	for header, value := range bodyMSG.Headers {
		if _, exists := msg.Headers[header]; !exists {
			value, err = url.QueryUnescape(value)
			if err != nil {
				return err
			}
			msg.Headers[header] = value
		}

	}
	if bodyMSG.Body != "" {
		msg.Body = bodyMSG.Body
	}
	return nil
}
