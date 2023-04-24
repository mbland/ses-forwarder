package handler

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ses"
)

type Handler struct {
	S3      *s3.Client
	Ses     *ses.Client
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

	if storedMsg, err := h.getStoredMessage(ctx, msgId); err != nil {
		return raiseErr(err)
	} else if fwdMsg, err := h.createForwardedMessage(storedMsg); err != nil {
		return raiseErr(err)
	} else if err := h.send(ctx, fwdMsg); err != nil {
		return raiseErr(err)
	} else {
		h.Log.Printf("successfully forwarded message %s", msgId)
	}

	return &events.SimpleEmailDisposition{
		Disposition: events.SimpleEmailStopRuleSet,
	}, nil
}

type storedMessage struct {
	body io.ReadCloser
	key  string
}

type forwardedMessage struct {
	// sender     string
	// recipients []string
	// content    string
}

func (h *Handler) getStoredMessage(
	ctx context.Context, msgId string,
) (msg *storedMessage, err error) {
	key := h.Options.IncomingPrefix + "/" + msgId
	input := &s3.GetObjectInput{Bucket: &h.Options.BucketName, Key: &key}
	var output *s3.GetObjectOutput

	if output, err = h.S3.GetObject(ctx, input); err != nil {
		return
	}
	msg = &storedMessage{output.Body, key}
	return
}

func (h *Handler) createForwardedMessage(
	msg *storedMessage,
) (fwdMsg *forwardedMessage, err error) {
	return
}

func (h *Handler) send(ctx context.Context, msg *forwardedMessage) error {
	return nil
}
