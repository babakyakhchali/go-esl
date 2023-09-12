package esl_test

import (
	"bufio"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/babakyakhchali/go-fsesl/esl"
	"github.com/google/uuid"
)

func onNewEvent(event esl.ESLMessage) {
	fmt.Printf("got new event:%s", event.Serialize())
}

func TestESL(t *testing.T) {

	esl := esl.NewInboundESLConnection(esl.ESLConfig{
		Host:         "127.0.0.1",
		Port:         8021,
		Password:     "ClueCon",
		EnableBgJOBs: true,
	})
	esl.Handlers["*"] = onNewEvent
	err := esl.Connect()
	if err != nil {
		t.Fatalf("connection failed, error:%s", err)
	}
	go esl.ReadMessages()
	_, err = esl.Authenticate()
	if err != nil {
		t.Fatalf("authentication failed, error:%s", err)
	}
	err = esl.InitEventBindings()
	if err != nil {
		t.Fatalf("event binding failed, error:%s", err)
	}

	result, err := esl.API("version", "")
	if err != nil {
		t.Fatalf("api version failed, error:%s", err)
	}
	if !strings.HasPrefix(result.Body, "Free") {
		t.Fatalf("api version bad result , body:%s", result.Body)
	}

	asyncResult, err := esl.BgAPIWithResult("version", "", 3*time.Second)
	if err != nil {
		t.Fatalf("bgapi version failed, error:%s", err)
	}
	if !strings.HasPrefix(asyncResult.SendResult.Body, "Free") {
		t.Fatalf("api version bad result , body:%s", result.Body)
	}
	time.Sleep(2 * time.Second)
	esl.CLose()
	esl.Wait()
}

func TestParser(t *testing.T) {
	eventString := `Content-Length: 674
Content-Type: text/event-plain

Event-Name: BACKGROUND_JOB
Core-UUID: ef96cd89-e2fe-4e30-813e-150327e2d03d
FreeSWITCH-Hostname: DESKTOP-D8K9BJT
FreeSWITCH-Switchname: DESKTOP-D8K9BJT
FreeSWITCH-IPv4: 172.21.86.241
FreeSWITCH-IPv6: %3A%3A1
Event-Date-Local: 2023-09-10%2013%3A05%3A18
Event-Date-GMT: Sun,%2010%20Sep%202023%2009%3A35%3A18%20GMT
Event-Date-Timestamp: 1694338518748101
Event-Calling-File: mod_commands.c
Event-Calling-Function: bgapi_exec
Event-Calling-Line-Number: 5395
Event-Sequence: 583
Job-UUID: e3b9f524-e20e-4996-adf9-30bb465cda68
Job-Command: version
Content-Length: 113

FreeSWITCH Version 1.10.8-release+git~20221014T193245Z~3510866140~64bit (git 3510866 2022-10-14 19:32:45Z 64bit)
`

	msg, err := esl.ParseESLStream(bufio.NewReader(strings.NewReader(eventString)))

	if err != nil {
		t.Fatalf("parsing test event string failed , error:%s", err)
	}

	if msg.ContentLength != 674 {
		t.Fatalf("failed parsing event string %s", msg.Body)
	}

	err = esl.MergeEventBody(&msg)
	if err != nil {
		t.Fatalf("error merging body, error:%s", err)
	}
	if msg.Headers["Event-Name"] != "BACKGROUND_JOB" {
		t.Fatalf("failed parsing event %v", msg.Headers)
	}

}

func TestOutbound(t *testing.T) {
	con := esl.NewInboundESLConnection(esl.ESLConfig{
		Host:         "127.0.0.1",
		Port:         8021,
		Password:     "ClueCon",
		EnableBgJOBs: true,
	})

	err := con.Init()
	if err != nil {
		t.Fatalf("failed initializing esl, error:%s", err)
	}

	fakeSessionID := uuid.NewString()
	res, err := con.Execute(esl.ExecutionOptions{
		App:         "set",
		Args:        "a=b",
		ChannelUUID: fakeSessionID,
		Lock:        true,
		Loops:       1,
		AppUUID:     "",
		Timeout:     3,
	}) //"set", "", fakeSessionID, true, 1, uuid.NewString()

	if err != nil {
		t.Fatalf("outbound esl, error:%s", err)
	}

	if res.SendResult.GetReply() != fmt.Sprintf("-ERR invalid session id [%s]", fakeSessionID) {
		t.Fatalf("outbound esl result missmatch reply:%s", res.SendResult.GetReply())
	}

}
