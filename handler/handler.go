package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/mail"

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

	if orig, err := h.getOriginalMessage(ctx, key); err != nil {
		return raiseErr(err)
	} else if updated, err := h.updateMessage(orig, key); err != nil {
		return raiseErr(err)
	} else if fwdId, err := h.forwardMessage(ctx, updated); err != nil {
		return raiseErr(err)
	} else {
		h.Log.Printf("successfully forwarded message %s as %s", key, fwdId)
	}

	return &events.SimpleEmailDisposition{
		Disposition: events.SimpleEmailStopRuleSet,
	}, nil
}

func (h *Handler) getOriginalMessage(
	ctx context.Context, key string,
) (msg []byte, err error) {
	input := &s3.GetObjectInput{Bucket: &h.Options.BucketName, Key: &key}
	var output *s3.GetObjectOutput

	if output, err = h.S3.GetObject(ctx, input); err == nil {
		msg, err = io.ReadAll(output.Body)
		output.Body.Close()
	}
	return
}

func (h *Handler) updateMessage(msg []byte, key string) ([]byte, error) {
	m, err := mail.ReadMessage(bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}

	keepHeaders := map[string]bool{
		"From":         true,
		"To":           true,
		"Cc":           true,
		"Bcc":          true,
		"Subject":      true,
		"Reply-To":     true,
		"Content-Type": true,
		"MIME-Version": true,
		"Mime-Version": true,
	}

	origFrom := m.Header.Get("From")
	newFrom, err := h.newFromAddress(origFrom)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	emitReplyTo := false

	for header, values := range m.Header {
		if !keepHeaders[header] {
			continue
		} else if header == "From" {
			values = []string{newFrom}
			emitReplyTo = m.Header.Get("Reply-To") == ""
		} else if header == "Mime-Version" {
			header = "MIME-Version"
		}
		for _, value := range values {
			b.Write([]byte(header))
			b.Write([]byte(": "))
			b.Write([]byte(value))
			b.Write([]byte("\r\n"))
		}
		if emitReplyTo {
			b.Write([]byte("Reply-To: "))
			b.Write([]byte(origFrom))
			b.Write([]byte("\r\n"))
			emitReplyTo = false
		}
	}
	b.Write([]byte("\r\n"))

	body, err := io.ReadAll(m.Body)
	if err != nil {
		return nil, err
	}
	b.Write(body)
	return b.Bytes(), nil
}

func (h *Handler) newFromAddress(oldFrom string) (string, error) {
	fromAddr, err := mail.ParseAddress(oldFrom)
	if err != nil {
		return "", err
	}

	const newFromFmt = "%s at %s <%s>"
	return fmt.Sprintf(
		newFromFmt, fromAddr.Name, fromAddr.Address, h.Options.SenderAddress,
	), nil
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

	if output, err = h.Ses.SendRawEmail(ctx, sesMsg); err == nil {
		forwardedMessageId = *output.MessageId
	}
	return
}
