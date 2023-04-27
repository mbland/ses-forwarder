//go:build small_tests || all_tests

package handler

import (
	"errors"
	"io"
	"net/mail"
	"strings"
	"testing"

	"gotest.tools/assert"
)

func newHeaderBuffer() (*strings.Builder, *headerBuffer) {
	builder := &strings.Builder{}
	return builder, &headerBuffer{builder, nil}
}

func TestWrite(t *testing.T) {
	t.Run("Succeeds", func(t *testing.T) {
		result, hb := newHeaderBuffer()

		hb.write("foobar")

		assert.NilError(t, hb.err)
		assert.Equal(t, result.String(), "foobar")
	})

	t.Run("WritesNothingIfErrIsNotNil", func(t *testing.T) {
		result, hb := newHeaderBuffer()
		hb.err = errors.New("error from an earlier write")

		hb.write("foobar")

		assert.Equal(t, result.String(), "")
	})
}

func TestWriteHearder(t *testing.T) {
	t.Run("Succeeds", func(t *testing.T) {
		result, hb := newHeaderBuffer()

		hb.writeHeader("X-Test-Header", []string{"foo", "bar", "baz"})

		assert.NilError(t, hb.err)
		expected := "X-Test-Header: foo\r\n" +
			"X-Test-Header: bar\r\n" +
			"X-Test-Header: baz\r\n"
		assert.Equal(t, result.String(), expected)
	})

	t.Run("CapitalizesMIMEVersion", func(t *testing.T) {
		result, hb := newHeaderBuffer()

		hb.writeHeader("Mime-Version", []string{"1.0"})

		assert.NilError(t, hb.err)
		assert.Equal(t, result.String(), "MIME-Version: 1.0\r\n")
	})
}

func TestNewFromAddress(t *testing.T) {
	senderAddress := "ses-forwarder@foo.com"

	t.Run("Succeeds", func(t *testing.T) {
		newFrom, err := newFromAddress(
			"Mike Bland <mbland@acm.org>", senderAddress,
		)

		assert.NilError(t, err)
		expected := "Mike Bland - mbland at acm.org <ses-forwarder@foo.com>"
		assert.Equal(t, expected, newFrom)
	})

	t.Run("SucceedsWhenAddressOnly", func(t *testing.T) {
		newFrom, err := newFromAddress("mbland@acm.org", senderAddress)

		assert.NilError(t, err)
		expected := "mbland at acm.org <ses-forwarder@foo.com>"
		assert.Equal(t, expected, newFrom)

	})

	t.Run("FailsIfOriginalFromMalformed", func(t *testing.T) {
		const addr = "Mike Bland mbland@acm.org"

		newFrom, err := newFromAddress(addr, senderAddress)

		assert.Equal(t, "", newFrom)
		assert.ErrorContains(t, err, "couldn't parse From address "+addr)
	})
}

func TestWriteFromAndReplyTo(t *testing.T) {
	t.Run("Succeeds", func(t *testing.T) {
		result, hb := newHeaderBuffer()
		headers := mail.Header{"From": []string{"Mike <mbland@acm.org>"}}

		hb.writeFromAndReplyTo(headers, "foo@bar.com")

		assert.NilError(t, hb.err)
		expected := "From: Mike - mbland at acm.org <foo@bar.com>\r\n" +
			"Reply-To: Mike <mbland@acm.org>\r\n"
		assert.Equal(t, result.String(), expected)
	})

	t.Run("KeepsExistingReplyToHeader", func(t *testing.T) {
		result, hb := newHeaderBuffer()
		headers := mail.Header{
			"From":     []string{"Mike <mbland@acm.org>"},
			"Reply-To": []string{"xyzzy@plugh.com"},
		}

		hb.writeFromAndReplyTo(headers, "foo@bar.com")

		assert.NilError(t, hb.err)
		expected := "From: Mike - mbland at acm.org <foo@bar.com>\r\n" +
			"Reply-To: xyzzy@plugh.com\r\n"
		assert.Equal(t, result.String(), expected)
	})

	t.Run("SetsErrIfGettingFromAddressFails", func(t *testing.T) {
		result, hb := newHeaderBuffer()
		headers := mail.Header{"From": []string{"mbland AT acm.org"}}

		hb.writeFromAndReplyTo(headers, "foo@bar.com")

		assert.Equal(t, result.String(), "")
		assert.ErrorContains(t, hb.err, "mbland AT acm.org")
	})
}

type ErrWriter struct {
	buf              io.Writer
	errorOnSubstring string
}

func (w *ErrWriter) Write(b []byte) (int, error) {
	if strings.Contains(string(b), w.errorOnSubstring) {
		return 0, errors.New("found: " + w.errorOnSubstring)
	}
	return w.buf.Write(b)
}

func TestWriteUpdatedHeaders(t *testing.T) {
	setup := func() (*updateHeadersInput, *strings.Builder, *headerBuffer) {
		input := &updateHeadersInput{
			headers:       mail.Header{},
			senderAddress: "foo@bar.com",
			msgPath:       "bar.com/incoming/msgId",
		}
		builder := &strings.Builder{}
		return input, builder, &headerBuffer{buf: builder}
	}

	t.Run("EmitsOnlySpecificHeadersInSpecificOrder", func(t *testing.T) {
		input, result, hb := setup()
		for name, value := range map[string]string{
			"Return-Path":  "<bounce@foo.com>",
			"Mime-Version": "1.0",
			"From":         "Mike <mbland@acm.org>",
			"Reply-To":     "Mike <some@other.com>",
			"Cc":           "foo@bar.com",
			"Bcc":          "bar@baz.com",
			"Date":         "Fri, 18 Sep 1970 12:45:00 +0000",
			"Subject":      "There's a reason why we unit test",
			"To":           "foo@xyzzy.com",
			"Content-Type": `multipart/alternative; boundary="random-string"`,
		} {
			input.headers[name] = []string{value}
		}

		err := hb.WriteUpdatedHeaders(input)

		assert.NilError(t, err)
		expected := strings.Join(
			[]string{
				"From: Mike - mbland at acm.org <foo@bar.com>",
				"Reply-To: Mike <some@other.com>",
				"To: foo@xyzzy.com",
				"Cc: foo@bar.com",
				"Bcc: bar@baz.com",
				"Subject: There's a reason why we unit test",
				"MIME-Version: 1.0",
				`Content-Type: multipart/alternative; boundary="random-string"`,
				origLinkHeaderPrefix + input.msgPath,
			},
			"\r\n",
		) + "\r\n\r\n"
		assert.Equal(t, result.String(), expected)
	})

	t.Run("ErrorsIfUpdatingAnyHeaderFailed", func(t *testing.T) {
		input, result, hb := setup()
		ew := &ErrWriter{result, "There's a reason why we unit test"}
		hb.buf = ew
		for name, value := range map[string]string{
			"From":    "Mike <mbland@acm.org>",
			"Date":    "Fri, 18 Sep 1970 12:45:00 +0000",
			"Subject": ew.errorOnSubstring,
			"To":      "foo@xyzzy.com",
		} {
			input.headers[name] = []string{value}
		}

		err := hb.WriteUpdatedHeaders(input)

		expectedErr := "error updating email headers: found: " +
			ew.errorOnSubstring
		assert.ErrorContains(t, err, expectedErr)

		expectedHeaders := "From: Mike - mbland at acm.org <foo@bar.com>\r\n" +
			"Reply-To: Mike <mbland@acm.org>\r\n" +
			"To: foo@xyzzy.com\r\n"
		assert.Equal(t, result.String(), expectedHeaders)
	})
}
