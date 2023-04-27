//go:build small_tests || all_tests

package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
)

type TestSes struct {
	rawEmailInput  *ses.SendRawEmailInput
	rawEmailOutput *ses.SendRawEmailOutput
	rawEmailErr    error
	bounceInput    *ses.SendBounceInput
	bounceOutput   *ses.SendBounceOutput
	bounceErr      error
}

func (ses *TestSes) SendRawEmail(
	ctx context.Context, input *ses.SendRawEmailInput, _ ...func(*ses.Options),
) (*ses.SendRawEmailOutput, error) {
	ses.rawEmailInput = input
	return ses.rawEmailOutput, ses.rawEmailErr
}

func (ses *TestSes) SendBounce(
	ctx context.Context, input *ses.SendBounceInput, _ ...func(*ses.Options),
) (*ses.SendBounceOutput, error) {
	ses.bounceInput = input
	return ses.bounceOutput, ses.bounceErr
}

type TestS3 struct {
	input                   *s3.GetObjectInput
	returnErrReaderInOutput bool
	outputMsg               []byte
	returnErr               error
}

func (testS3 *TestS3) GetObject(
	ctx context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options),
) (*s3.GetObjectOutput, error) {
	testS3.input = input
	var r io.Reader

	if testS3.returnErrReaderInOutput {
		r = &ErrReader{errors.New(string(testS3.outputMsg))}
	} else {
		r = bytes.NewReader(testS3.outputMsg)
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(r)}, testS3.returnErr
}

type ErrReader struct {
	err error
}

func (r *ErrReader) Read([]byte) (int, error) {
	return 0, r.err
}

type TestLogs = strings.Builder

func testLogger() (*TestLogs, *log.Logger) {
	builder := &TestLogs{}
	logger := log.New(builder, "test logger: ", 0)
	return builder, logger
}

func assertLogsContain(t *testing.T, tl *TestLogs, message string) {
	t.Helper()
	assert.Assert(t, is.Contains(tl.String(), message))
}

func TestBounceIfDmarcFails(t *testing.T) {
	recipient := "mbland@acm.org"
	bouncedId := "didBounce"

	setup := func() (
		*TestSes, *Handler, *events.SimpleEmailService, context.Context,
	) {
		testSes := &TestSes{
			bounceOutput: &ses.SendBounceOutput{MessageId: &bouncedId},
		}
		opts := &Options{EmailDomainName: "foo.com"}
		ctx := context.Background()
		sesInfo := &events.SimpleEmailService{
			Mail: events.SimpleEmailMessage{MessageID: "deadbeef"},
			Receipt: events.SimpleEmailReceipt{
				Recipients: []string{recipient},
			},
		}
		return testSes, &Handler{Ses: testSes, Options: opts}, sesInfo, ctx
	}

	t.Run("DoesNothingIfVerdictIsNotFail", func(t *testing.T) {
		testSes, h, sesInfo, ctx := setup()
		sesInfo.Receipt.DMARCVerdict.Status = "pass"
		sesInfo.Receipt.DMARCPolicy = "reject"

		bounceId, err := h.bounceIfDmarcFails(ctx, sesInfo)

		assert.NilError(t, err)
		assert.Equal(t, bounceId, "")
		assert.Assert(t, is.Nil(testSes.bounceInput))
	})

	t.Run("DoesNothingIfPolicyIsNotReject", func(t *testing.T) {
		testSes, h, sesInfo, ctx := setup()
		sesInfo.Receipt.DMARCVerdict.Status = "fail"
		sesInfo.Receipt.DMARCPolicy = "none"

		bounceId, err := h.bounceIfDmarcFails(ctx, sesInfo)

		assert.NilError(t, err)
		assert.Equal(t, bounceId, "")
		assert.Assert(t, is.Nil(testSes.bounceInput))
	})

	t.Run("BouncesIfVerdictFailsAndPolicyRejects", func(t *testing.T) {
		testSes, h, sesInfo, ctx := setup()
		sesInfo.Receipt.DMARCVerdict.Status = "fail"
		sesInfo.Receipt.DMARCPolicy = "reject"

		bounceId, err := h.bounceIfDmarcFails(ctx, sesInfo)

		assert.NilError(t, err)
		assert.Equal(t, bounceId, bouncedId)
		assert.Assert(t, testSes.bounceInput != nil)

		bouncedRecipients := testSes.bounceInput.BouncedRecipientInfoList
		assert.Equal(t, len(bouncedRecipients), 1)
		assert.Equal(t, *bouncedRecipients[0].Recipient, recipient)
		assert.Equal(
			t, bouncedRecipients[0].BounceType, types.BounceTypeContentRejected,
		)
	})

	t.Run("ErrorsIfSendBounceFails", func(t *testing.T) {
		testSes, h, sesInfo, ctx := setup()
		sesInfo.Receipt.DMARCVerdict.Status = "fail"
		sesInfo.Receipt.DMARCPolicy = "reject"
		testSes.bounceErr = errors.New("test error")

		bounceId, err := h.bounceIfDmarcFails(ctx, sesInfo)

		assert.Equal(t, bounceId, "")
		assert.Assert(t, testSes.bounceInput != nil)
		assert.ErrorContains(t, err, "DMARC bounce failed: test error")
	})
}

func TestIsSpam(t *testing.T) {
	failedVerdict := func(checkType string) *events.SimpleEmailService {
		sesInfo := &events.SimpleEmailService{}
		var verdict *events.SimpleEmailVerdict

		switch checkType {
		case "SPF":
			verdict = &sesInfo.Receipt.SPFVerdict
		case "DKIM":
			verdict = &sesInfo.Receipt.DKIMVerdict
		case "Spam":
			verdict = &sesInfo.Receipt.SpamVerdict
		case "Virus":
			verdict = &sesInfo.Receipt.VirusVerdict
		}

		if verdict != nil {
			verdict.Status = "fail"
		}
		return sesInfo
	}
	t.Run("ReturnsFalseIfNoVerdictsFail", func(t *testing.T) {
		assert.Assert(t, isSpam(failedVerdict("none")) == false)
	})

	t.Run("ReturnsTrueIfAnyVerdictFails", func(t *testing.T) {
		assert.Check(t, isSpam(failedVerdict("SPF")) == true)
		assert.Check(t, isSpam(failedVerdict("DKIM")) == true)
		assert.Check(t, isSpam(failedVerdict("Spam")) == true)
		assert.Assert(t, isSpam(failedVerdict("Virus")) == true)
	})
}

func TestValidateMessage(t *testing.T) {
	bouncedId := "didBounce"

	setup := func() (
		*TestSes, *Handler, *events.SimpleEmailService, context.Context,
	) {
		testSes := &TestSes{
			bounceOutput: &ses.SendBounceOutput{MessageId: &bouncedId},
		}
		opts := &Options{EmailDomainName: "foo.com"}
		ctx := context.Background()
		sesInfo := &events.SimpleEmailService{
			Mail: events.SimpleEmailMessage{MessageID: "deadbeef"},
		}
		return testSes, &Handler{Ses: testSes, Options: opts}, sesInfo, ctx
	}

	t.Run("Succeeds", func(t *testing.T) {
		_, h, sesInfo, ctx := setup()

		err := h.validateMessage(ctx, sesInfo)

		assert.NilError(t, err)
	})

	t.Run("ErrorsIfDmarcBounceFails", func(t *testing.T) {
		testSes, h, sesInfo, ctx := setup()
		sesInfo.Receipt.DMARCVerdict.Status = "fail"
		sesInfo.Receipt.DMARCPolicy = "reject"
		testSes.bounceErr = errors.New("test error")

		err := h.validateMessage(ctx, sesInfo)

		assert.ErrorContains(t, err, "DMARC bounce failed: test error")
	})

	t.Run("ErrorsIfMessageBounced", func(t *testing.T) {
		_, h, sesInfo, ctx := setup()
		sesInfo.Receipt.DMARCVerdict.Status = "fail"
		sesInfo.Receipt.DMARCPolicy = "reject"

		err := h.validateMessage(ctx, sesInfo)

		expected := "DMARC bounced with bounce ID: " + bouncedId
		assert.ErrorContains(t, err, expected)
	})

	t.Run("ErrorsIfIsSpam", func(t *testing.T) {
		_, h, sesInfo, ctx := setup()
		sesInfo.Receipt.SPFVerdict.Status = "fail"

		err := h.validateMessage(ctx, sesInfo)

		assert.ErrorContains(t, err, "marked as spam, ignoring")
	})
}

func TestGetOriginalMessage(t *testing.T) {
	setup := func() (*TestS3, *Handler, context.Context) {
		testS3 := &TestS3{}
		opts := &Options{BucketName: "mail.foo.com"}
		ctx := context.Background()
		return testS3, &Handler{S3: testS3, Options: opts}, ctx
	}

	t.Run("Succeeds", func(t *testing.T) {
		testS3, h, ctx := setup()
		testS3.outputMsg = []byte("Hello, world!")
		msg, err := h.getOriginalMessage(ctx, "prefix/msgId")

		assert.NilError(t, err)
		assert.Equal(t, "Hello, world!", string(msg))
		assert.Equal(t, h.Options.BucketName, *testS3.input.Bucket)
		assert.Equal(t, "prefix/msgId", *testS3.input.Key)
	})

	t.Run("ErrorsIfGetObjectFails", func(t *testing.T) {
		testS3, h, ctx := setup()
		testS3.returnErr = errors.New("S3 test error")

		msg, err := h.getOriginalMessage(ctx, "prefix/msgId")

		assert.Equal(t, string(msg), "")
		expected := "failed to get original message: S3 test error"
		assert.ErrorContains(t, err, expected)
	})

	t.Run("ErrorsIfReadingBodyFails", func(t *testing.T) {
		testS3, h, ctx := setup()
		testS3.returnErrReaderInOutput = true
		testS3.outputMsg = []byte("test read error")

		msg, err := h.getOriginalMessage(ctx, "prefix/msgId")

		assert.Equal(t, string(msg), "")
		expected := "failed to get original message: test read error"
		assert.ErrorContains(t, err, expected)
	})
}

func TestForwardMessage(t *testing.T) {
	var forwardedMsgId string = "forwardedMsgId"

	setup := func() (*TestSes, *Handler, context.Context) {
		testSes := &TestSes{rawEmailOutput: &ses.SendRawEmailOutput{}}
		opts := &Options{
			ForwardingAddress: "quux@xyzzy.com",
			ConfigurationSet:  "ses-forwarder",
		}
		ctx := context.Background()
		return testSes, &Handler{Ses: testSes, Options: opts}, ctx
	}

	t.Run("Succeeds", func(t *testing.T) {
		testSes, h, ctx := setup()
		testSes.rawEmailOutput.MessageId = &forwardedMsgId
		fwdAddr := h.Options.ForwardingAddress
		configSet := h.Options.ConfigurationSet
		msg := []byte("Hello, world!")

		fwdId, err := h.forwardMessage(ctx, msg)

		assert.NilError(t, err)
		assert.Equal(t, forwardedMsgId, fwdId)
		assert.DeepEqual(t, []string{fwdAddr}, testSes.rawEmailInput.Destinations)
		assert.Equal(t, configSet, *testSes.rawEmailInput.ConfigurationSetName)
		assert.DeepEqual(t, msg, testSes.rawEmailInput.RawMessage.Data)
	})

	t.Run("ErrorsIfSendingFails", func(t *testing.T) {
		testSes, h, ctx := setup()
		testSes.rawEmailErr = errors.New("SES test error")

		fwdId, err := h.forwardMessage(ctx, []byte("Hello, world!"))

		assert.Equal(t, "", fwdId)
		assert.ErrorContains(t, err, "send failed: SES test error")
	})
}

var beforeHeaders string = strings.Join([]string{
	`Return-Path: <bounce@foo.com>`,
	`Received: ...`,
	` by ...`,
	`X-SES-Spam-Verdict: PASS`,
	`MIME-Version: 1.0`,
	`From: Mike Bland <mbland@acm.org>`,
	`Cc: foo@bar.com`,
	`Bcc: bar@baz.com`,
	`Date: Fri, 18 Sep 1970 12:45:00 +0000`,
	`Message-ID: <...>`,
	`Subject: There's a reason why we unit test`,
	`To: foo@xyzzy.com`,
	`Content-Type: multipart/alternative; boundary="random-string"`,
}, "\r\n")

var msgBody string = strings.Join([]string{
	`--random-string`,
	`Content-Type: text/plain; charset="UTF-8"`,
	``,
	`Sometimes the getting smallest detail wrong breaks everything.`,
	``,
	`--random-string`,
	`Content-Type: text/html; charset="UTF-8"`,
	``,
	`<div dir="ltr">Sometimes the getting smallest detail wrong`,
	`breaks everything.</div>`,
	``,
	`--random-string--`,
}, "\r\n")

var testMsg []byte = []byte(beforeHeaders + "\r\n\r\n" + msgBody)

func TestUpdateMessage(t *testing.T) {
	setup := func() (*Handler, *Options) {
		opts := &Options{
			BucketName:        "xyzzy.com",
			SenderAddress:     "ses-updater@xyzzy.com",
			ForwardingAddress: "quux@xyzzy.com",
			ConfigurationSet:  "ses-forwarder",
		}
		return &Handler{Options: opts}, opts
	}

	t.Run("Succeeds", func(t *testing.T) {
		h, opts := setup()
		msgKey := "prefix/msgId"

		result, err := h.updateMessage(testMsg, msgKey)

		assert.NilError(t, err)
		// The headers appear in the same order as keepHeaders.
		expected := strings.Join([]string{
			`From: Mike Bland - mbland at acm.org <` + opts.SenderAddress + `>`,
			`Reply-To: Mike Bland <mbland@acm.org>`,
			`To: foo@xyzzy.com`,
			`Cc: foo@bar.com`,
			`Bcc: bar@baz.com`,
			`Subject: There's a reason why we unit test`,
			`MIME-Version: 1.0`,
			`Content-Type: multipart/alternative; boundary="random-string"`,
			`X-SES-Forwarder-Original: s3://` + opts.BucketName + `/` + msgKey,
			``,
			msgBody,
		}, "\r\n")
		assert.Equal(t, expected, string(result))
	})

	t.Run("ErrorsIfReadingMessageFails", func(t *testing.T) {
		h, _ := setup()

		result, err := h.updateMessage([]byte("not an email"), "prefix/msgId")

		assert.Equal(t, string(result), "")
		assert.ErrorContains(t, err, "failed to parse message: ")
	})

	t.Run("ErrorsIfUpdatingHeadersFails", func(t *testing.T) {
		h, _ := setup()
		badMsg := []byte("From: D'oh!\r\n\r\nThis is only a test.\r\n")

		result, err := h.updateMessage(badMsg, "prefix/msgId")

		assert.Equal(t, string(result), "")
		expected := "error updating email headers: " +
			"couldn't parse From address D'oh!:"
		assert.ErrorContains(t, err, expected)
	})
}

type handleEventFixture struct {
	s3          *TestS3
	ses         *TestSes
	event       *events.SimpleEmailEvent
	forwardedId string
	logs        *TestLogs
	h           *Handler
}

func newHandleEventFixture() *handleEventFixture {
	forwardedId := "fwd-msg-id"
	testS3 := &TestS3{outputMsg: testMsg}
	testSes := &TestSes{
		rawEmailOutput: &ses.SendRawEmailOutput{
			MessageId: &forwardedId,
		},
	}
	logs, logger := testLogger()
	opts := &Options{
		BucketName:        "mail.bar.com",
		IncomingPrefix:    "incoming",
		EmailDomainName:   "bar.com",
		SenderAddress:     "mbland@acm.org",
		ForwardingAddress: "foo@bar.com",
		ConfigurationSet:  "bar.com",
	}
	h := &Handler{testS3, testSes, opts, logger}
	event := &events.SimpleEmailEvent{
		Records: []events.SimpleEmailRecord{
			{
				SES: events.SimpleEmailService{
					Mail:    events.SimpleEmailMessage{MessageID: "deadbeef"},
					Receipt: events.SimpleEmailReceipt{},
				},
			},
		},
	}
	return &handleEventFixture{testS3, testSes, event, forwardedId, logs, h}
}

func TestProcessMesssage(t *testing.T) {
	setup := func() (
		f *handleEventFixture,
		sesInfo *events.SimpleEmailService,
		msgKey string,
		ctx context.Context,
	) {
		f = newHandleEventFixture()
		return f,
			&f.event.Records[0].SES,
			f.h.Options.IncomingPrefix + "/deadbeef",
			context.Background()
	}

	errMsg := func(msgKey, message string) string {
		return "failed to forward message " + msgKey + ": " + message
	}

	t.Run("Succeeds", func(t *testing.T) {
		f, sesInfo, msgKey, ctx := setup()

		f.h.processMessage(ctx, sesInfo)

		assertLogsContain(t, f.logs, "forwarding message "+msgKey)
		successLogMsg := "successfully forwarded message " + msgKey +
			" as " + f.forwardedId
		assertLogsContain(t, f.logs, successLogMsg)
	})

	t.Run("ErrorsIfValidationFails", func(t *testing.T) {
		f, sesInfo, msgKey, ctx := setup()
		sesInfo.Receipt.SpamVerdict.Status = "FAIL"

		f.h.processMessage(ctx, sesInfo)

		assertLogsContain(t, f.logs, errMsg(msgKey, "marked as spam, ignoring"))
	})

	t.Run("ErrorsIfGettingOriginalFails", func(t *testing.T) {
		f, sesInfo, msgKey, ctx := setup()
		f.s3.returnErr = errors.New("s3 error")

		f.h.processMessage(ctx, sesInfo)

		expected := errMsg(msgKey, "failed to get original message: s3 error")
		assertLogsContain(t, f.logs, expected)
	})

	t.Run("ErrorsIfUpdatingMessageFails", func(t *testing.T) {
		f, sesInfo, msgKey, ctx := setup()
		f.s3.outputMsg = []byte("invalid message")

		f.h.processMessage(ctx, sesInfo)

		expected := errMsg(msgKey, "failed to parse message: ")
		assertLogsContain(t, f.logs, expected)
	})

	t.Run("ErrorsIfForwardingMessageFails", func(t *testing.T) {
		f, sesInfo, msgKey, ctx := setup()
		f.ses.rawEmailErr = errors.New("SES error")

		f.h.processMessage(ctx, sesInfo)

		expected := errMsg(msgKey, "send failed: SES error")
		assertLogsContain(t, f.logs, expected)
	})
}

func TestHandleEvent(t *testing.T) {
	setup := func() (
		f *handleEventFixture, msgKey string, ctx context.Context,
	) {
		f = newHandleEventFixture()
		msgKey = f.h.Options.IncomingPrefix + "/deadbeef"
		ctx = context.Background()
		return
	}

	assertSuccessLogs := func(
		t *testing.T, f *handleEventFixture, msgKey string,
	) {
		t.Helper()
		assertLogsContain(t, f.logs, "forwarding message "+msgKey)
		successLogMsg := "successfully forwarded message " + msgKey +
			" as " + f.forwardedId
		assertLogsContain(t, f.logs, successLogMsg)
	}

	t.Run("Succeeds", func(t *testing.T) {
		f, msgKey, ctx := setup()

		result, err := f.h.HandleEvent(ctx, f.event)

		assert.NilError(t, err)
		assert.Equal(t, result.Disposition, events.SimpleEmailStopRuleSet)
		assertSuccessLogs(t, f, msgKey)
	})

	t.Run("HandlesMultipleEvents", func(t *testing.T) {
		f, msgKey, ctx := setup()
		f.event.Records = append(f.event.Records, events.SimpleEmailRecord{
			SES: events.SimpleEmailService{
				Mail:    events.SimpleEmailMessage{MessageID: "beefdead"},
				Receipt: events.SimpleEmailReceipt{},
			},
		})

		result, err := f.h.HandleEvent(ctx, f.event)

		assert.NilError(t, err)
		assert.Equal(t, result.Disposition, events.SimpleEmailStopRuleSet)
		assertSuccessLogs(t, f, msgKey)
		assertSuccessLogs(t, f, f.h.Options.IncomingPrefix+"/beefdead")
	})

	t.Run("ErrorsIfNoRecordsInEvent", func(t *testing.T) {
		f, _, ctx := setup()
		f.event.Records = []events.SimpleEmailRecord{}

		result, err := f.h.HandleEvent(ctx, f.event)

		assert.Assert(t, is.Nil(result))
		assert.ErrorContains(t, err, "SES event contained no records: ")
	})
}
