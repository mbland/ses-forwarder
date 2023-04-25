package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/mail"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
)

type S3Api interface {
	GetObject(
		context.Context, *s3.GetObjectInput, ...func(*s3.Options),
	) (*s3.GetObjectOutput, error)
}

type SesApi interface {
	SendRawEmail(
		context.Context, *ses.SendRawEmailInput, ...func(*ses.Options),
	) (*ses.SendRawEmailOutput, error)
}

type Handler struct {
	S3      S3Api
	Ses     SesApi
	Options *Options
	Log     *log.Logger
}

func (h *Handler) HandleEvent(
	ctx context.Context, e *events.SimpleEmailEvent,
) (*events.SimpleEmailDisposition, error) {
	if len(e.Records) == 0 {
		return nil, fmt.Errorf("SES event contained no records: %+v", e)
	}

	sesInfo := &e.Records[0].SES
	key := h.Options.IncomingPrefix + "/" + sesInfo.Mail.MessageID
	logErr := func(err error) {
		h.Log.Printf("failed to forward message %s: %s", key, err)
	}

	h.Log.Printf("forwarding message %s", key)

	if err := h.validateMessage(sesInfo); err != nil {
		logErr(err)
	} else if orig, err := h.getOriginalMessage(ctx, key); err != nil {
		logErr(err)
	} else if updated, err := h.updateMessage(orig, key); err != nil {
		logErr(err)
	} else if fwdId, err := h.forwardMessage(ctx, updated); err != nil {
		logErr(err)
	} else {
		h.Log.Printf("successfully forwarded message %s as %s", key, fwdId)
	}

	return &events.SimpleEmailDisposition{
		Disposition: events.SimpleEmailStopRuleSet,
	}, nil
}

func (h *Handler) validateMessage(info *events.SimpleEmailService) error {
	if bounceId, err := h.bounceIfDmarcFails(info); err != nil {
		return err
	} else if bounceId != "" {
		return errors.New("DMARC bounced with bounce ID: " + bounceId)
	} else if isSpam(info) {
		return errors.New("marked as spam, ignoring")
	}
	return nil
}

func (h *Handler) bounceIfDmarcFails(
	info *events.SimpleEmailService,
) (string, error) {
	return "", nil
}

func isSpam(info *events.SimpleEmailService) bool {
	return false
}
func (h *Handler) getOriginalMessage(
	ctx context.Context, key string,
) (msg []byte, err error) {
	input := &s3.GetObjectInput{Bucket: &h.Options.BucketName, Key: &key}
	var output *s3.GetObjectOutput

	if output, err = h.S3.GetObject(ctx, input); err == nil {
		msg, err = io.ReadAll(output.Body)
	}
	if err != nil {
		err = fmt.Errorf("failed to get original message: %s", err)
	}
	return
}

func (h *Handler) updateMessage(msg []byte, key string) ([]byte, error) {
	m, err := mail.ReadMessage(bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}

	b := &bytes.Buffer{}
	hb := headerBuffer{
		b, m.Header, h.Options.SenderAddress, h.Options.BucketName, key, nil,
	}

	if err = hb.WriteUpdatedHeaders(); err != nil {
		return nil, err
	} else if _, err = b.ReadFrom(m.Body); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (h *Handler) forwardMessage(
	ctx context.Context, msg []byte,
) (forwardedMessageId string, err error) {
	sesMsg := &ses.SendRawEmailInput{
		Destinations:         []string{h.Options.ForwardingAddress},
		ConfigurationSetName: &h.Options.ConfigurationSet,
		RawMessage:           &types.RawMessage{Data: msg},
	}
	var output *ses.SendRawEmailOutput

	if output, err = h.Ses.SendRawEmail(ctx, sesMsg); err != nil {
		err = fmt.Errorf("send failed: %s", err)
	} else {
		forwardedMessageId = *output.MessageId
	}
	return
}
