package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/mail"
	"strings"
	"time"

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

	SendBounce(
		context.Context, *ses.SendBounceInput, ...func(*ses.Options),
	) (*ses.SendBounceOutput, error)
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
	h.processMessage(sesInfo, ctx)

	return &events.SimpleEmailDisposition{
		Disposition: events.SimpleEmailStopRuleSet,
	}, nil
}

func (h *Handler) processMessage(
	sesInfo *events.SimpleEmailService, ctx context.Context,
) {
	key := h.Options.IncomingPrefix + "/" + sesInfo.Mail.MessageID
	logErr := func(err error) {
		h.Log.Printf("failed to forward message %s: %s", key, err)
	}

	h.Log.Printf("forwarding message %s", key)

	if err := h.validateMessage(ctx, sesInfo); err != nil {
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
}

func (h *Handler) validateMessage(
	ctx context.Context, info *events.SimpleEmailService,
) error {
	if bounceId, err := h.bounceIfDmarcFails(ctx, info); err != nil {
		return err
	} else if bounceId != "" {
		return errors.New("DMARC bounced with bounce ID: " + bounceId)
	} else if isSpam(info) {
		return errors.New("marked as spam, ignoring")
	}
	return nil
}

// https://docs.aws.amazon.com/ses/latest/dg/receiving-email-action-lambda-example-functions.html
func (h *Handler) bounceIfDmarcFails(
	ctx context.Context, info *events.SimpleEmailService,
) (bounceMessageId string, err error) {
	verdict := strings.ToUpper(info.Receipt.DMARCVerdict.Status)
	policy := strings.ToUpper(info.Receipt.DMARCPolicy)

	if verdict != "FAIL" || policy != "REJECT" {
		return
	}

	sender := "mailer-daemon@" + h.Options.EmailDomainName
	recipients := info.Receipt.Recipients
	recipientInfo := make([]types.BouncedRecipientInfo, len(recipients))
	reportingMta := "dns; " + h.Options.EmailDomainName
	arrivalDate := time.Now().Truncate(time.Second)
	explanation := "Unauthenticated email is not accepted due to " +
		"the sending domain's DMARC policy."

	for i, recipient := range recipients {
		recipientInfo[i].Recipient = &recipient
		recipientInfo[i].BounceType = types.BounceTypeContentRejected
	}

	input := &ses.SendBounceInput{
		BounceSender:      &sender,
		OriginalMessageId: &info.Mail.MessageID,
		MessageDsn: &types.MessageDsn{
			ReportingMta: &reportingMta,
			ArrivalDate:  &arrivalDate,
		},
		Explanation:              &explanation,
		BouncedRecipientInfoList: recipientInfo,
	}
	var output *ses.SendBounceOutput

	if output, err = h.Ses.SendBounce(ctx, input); err != nil {
		err = fmt.Errorf("DMARC bounce failed: %s", err)
	} else {
		bounceMessageId = *output.MessageId
	}
	return
}

// https://docs.aws.amazon.com/ses/latest/dg/receiving-email-action-lambda-example-functions.html
func isSpam(info *events.SimpleEmailService) bool {
	receipt := &info.Receipt
	return strings.ToUpper(receipt.SPFVerdict.Status) == "FAIL" ||
		strings.ToUpper(receipt.DKIMVerdict.Status) == "FAIL" ||
		strings.ToUpper(receipt.SpamVerdict.Status) == "FAIL" ||
		strings.ToUpper(receipt.VirusVerdict.Status) == "FAIL"
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
		return nil, fmt.Errorf("failed to parse message: %s", err)
	}

	b := &bytes.Buffer{}
	hb := headerBuffer{buf: b}
	input := &updateHeadersInput{
		m.Header, h.Options.SenderAddress, h.Options.BucketName + "/" + key,
	}

	if err = hb.WriteUpdatedHeaders(input); err != nil {
		return nil, err
	}

	// mail.ReadMessage practically guararantees the next line will succeed. The
	// only way it could fail is if the buffer runs out of memory, and ReadFrom
	// will panic in that case anyway.
	b.ReadFrom(m.Body)
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
