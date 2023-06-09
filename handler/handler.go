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
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sesv2types "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type S3Api interface {
	GetObject(
		context.Context, *s3.GetObjectInput, ...func(*s3.Options),
	) (*s3.GetObjectOutput, error)
}

type SesApi interface {
	SendBounce(
		context.Context, *ses.SendBounceInput, ...func(*ses.Options),
	) (*ses.SendBounceOutput, error)
}

type SesV2Api interface {
	SendEmail(
		context.Context, *sesv2.SendEmailInput, ...func(*sesv2.Options),
	) (*sesv2.SendEmailOutput, error)
}

type Handler struct {
	S3      S3Api
	Ses     SesApi
	SesV2   SesV2Api
	Options *Options
	Log     *log.Logger
}

func (h *Handler) HandleEvent(
	ctx context.Context, e *events.SimpleEmailEvent,
) (*events.SimpleEmailDisposition, error) {
	if len(e.Records) == 0 {
		return nil, fmt.Errorf("SES event contained no records: %+v", e)
	}

	for i := range e.Records {
		h.processMessage(ctx, &e.Records[i].SES)
	}

	return &events.SimpleEmailDisposition{
		Disposition: events.SimpleEmailStopRuleSet,
	}, nil
}

func (h *Handler) processMessage(
	ctx context.Context, sesInfo *events.SimpleEmailService,
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

	recipients := info.Receipt.Recipients
	recipientInfo := make([]sestypes.BouncedRecipientInfo, len(recipients))

	for i, recipient := range recipients {
		recipientInfo[i].Recipient = aws.String(recipient)
		recipientInfo[i].BounceType = sestypes.BounceTypeContentRejected
	}

	input := &ses.SendBounceInput{
		BounceSender: aws.String(
			"mailer-daemon@" + h.Options.EmailDomainName,
		),
		OriginalMessageId: aws.String(info.Mail.MessageID),
		MessageDsn: &sestypes.MessageDsn{
			ReportingMta: aws.String("dns; " + h.Options.EmailDomainName),
			ArrivalDate:  aws.Time(time.Now().Truncate(time.Second)),
		},
		Explanation: aws.String(
			"Unauthenticated email is not accepted due to " +
				"the sending domain's DMARC policy.",
		),
		BouncedRecipientInfoList: recipientInfo,
	}
	var output *ses.SendBounceOutput

	if output, err = h.Ses.SendBounce(ctx, input); err != nil {
		err = fmt.Errorf("DMARC bounce failed: %s", err)
	} else {
		bounceMessageId = aws.ToString(output.MessageId)
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
	input := &s3.GetObjectInput{
		Bucket: aws.String(h.Options.BucketName), Key: aws.String(key),
	}
	var output *s3.GetObjectOutput

	if output, err = h.S3.GetObject(ctx, input); err == nil {
		defer output.Body.Close()
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
	sesMsg := &sesv2.SendEmailInput{
		ConfigurationSetName: aws.String(h.Options.ConfigurationSet),
		Content: &sesv2types.EmailContent{
			Raw: &sesv2types.RawMessage{Data: msg},
		},
		Destination: &sesv2types.Destination{
			ToAddresses: []string{h.Options.ForwardingAddress},
		},
	}
	var output *sesv2.SendEmailOutput

	if output, err = h.SesV2.SendEmail(ctx, sesMsg); err != nil {
		err = fmt.Errorf("send failed: %s", err)
	} else {
		forwardedMessageId = aws.ToString(output.MessageId)
	}
	return
}
