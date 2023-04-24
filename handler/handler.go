package handler

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
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

	key := h.Options.IncomingPrefix + "/" + e.Records[0].SES.Mail.MessageID
	h.Log.Printf("forwarding message %s", key)
	raiseErr := func(err error) (*events.SimpleEmailDisposition, error) {
		return nil, fmt.Errorf("failed to forward message %s: %s", key, err)
	}

	if storedMsg, err := h.getStoredMessage(ctx, key); err != nil {
		return raiseErr(err)
	} else if fwdId, err := h.forwardMessage(ctx, storedMsg); err != nil {
		return raiseErr(err)
	} else {
		h.Log.Printf("successfully forwarded message %s as %s", key, fwdId)
	}

	return &events.SimpleEmailDisposition{
		Disposition: events.SimpleEmailStopRuleSet,
	}, nil
}

func (h *Handler) getStoredMessage(
	ctx context.Context, key string,
) (msg io.ReadCloser, err error) {
	input := &s3.GetObjectInput{Bucket: &h.Options.BucketName, Key: &key}
	var output *s3.GetObjectOutput

	if output, err = h.S3.GetObject(ctx, input); err == nil {
		msg = output.Body
	}
	return
}

func (h *Handler) forwardMessage(
	ctx context.Context, msg io.ReadCloser,
) (forwardedMessageId string, err error) {
	defer msg.Close()
	var rawMsg types.RawMessage

	if _, err = msg.Read(rawMsg.Data); err != nil {
		return
	}

	sesMsg := &ses.SendRawEmailInput{
		Destinations:         []string{h.Options.ForwardingAddress},
		ConfigurationSetName: &h.Options.ConfigurationSet,
		RawMessage:           &types.RawMessage{},
	}
	var output *ses.SendRawEmailOutput

	if output, err = h.Ses.SendRawEmail(ctx, sesMsg); err == nil {
		forwardedMessageId = *output.MessageId
	}
	return
}
