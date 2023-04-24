package handler

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
)

type Handler struct {
	Config  *aws.Config
	Options *Options
	Log     *log.Logger
}

func (h *Handler) HandleEvent(
	ctx context.Context, e *events.SimpleEmailEvent,
) (*events.SimpleEmailDisposition, error) {
	if len(e.Records) == 0 {
		return nil, fmt.Errorf("SES event contained no records: %+v", e)
	}

	msgId := e.Records[0].SES.Mail.MessageID
	h.Log.Printf("forwarding message %s", msgId)
	raiseErr := func(err error) (*events.SimpleEmailDisposition, error) {
		return nil, fmt.Errorf("failed to forward message %s: %s", msgId, err)
	}

	if storedMsg, err := h.getStoredMessage(msgId); err != nil {
		return raiseErr(err)
	} else if fwdMsg, err := h.createForwardedMessage(storedMsg); err != nil {
		return raiseErr(err)
	} else if err := h.send(fwdMsg); err != nil {
		return raiseErr(err)
	} else {
		h.Log.Printf("successfully forwarded message %s", msgId)
	}

	return &events.SimpleEmailDisposition{
		Disposition: events.SimpleEmailStopRuleSet,
	}, nil
}

type storedMessage struct {
	// content string
	// path    string
}

type forwardedMessage struct {
	// sender     string
	// recipients []string
	// content    string
}

func (h *Handler) getStoredMessage(msgId string) (*storedMessage, error) {
	return nil, nil
}

func (h *Handler) createForwardedMessage(
	msg *storedMessage,
) (*forwardedMessage, error) {
	return nil, nil
}

func (h *Handler) send(msg *forwardedMessage) error {
	return nil
}
