package esl

const (
	ContentTypeTextDisconnectNotice = "text/disconnect-notice"
	ContentTypeTextEventPlain       = "text/event-plain"
	ContentTypeTextEventJSON        = "text/event-json"
	ContentTypeTextEventXML         = "text/event-xml"
	ContentTypeCommandReply         = "command/reply"
	ContentTypeApiResponse          = "api/response"
	ContentTypeAuthRequest          = "auth/request"
	ContentTypeLogData              = "log/data"
)

const (
	MessageHeaderContentLength   = "Content-Length"
	MessageHeaderContentType     = "Content-Type"
	MessageHeaderReplyText       = "Reply-Text"
	MessageHeaderEventName       = "Event-Name"
	MessageHeaderEventSubclass   = "Event-Subclass"
	MessageHeaderJobUuid         = "Job-UUID"
	MessageHeaderUniqueID        = "Unique-ID"
	MessageHeaderApplicationUUID = "Application-UUID"
)

const (
	MessageLogFormatFull  = "full"
	MessageLogFormatBrief = "brief"
)
