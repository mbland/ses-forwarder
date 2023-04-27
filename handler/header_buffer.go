package handler

import (
	"fmt"
	"io"
	"net/mail"
	"strings"
)

type headerBuffer struct {
	buf io.Writer
	err error
}

type updateHeadersInput struct {
	headers       mail.Header
	senderAddress string
	msgPath       string
}

var keepHeaders = []string{
	"To",
	"Cc",
	"Bcc",
	"Subject",
	"Mime-Version",
	"Content-Type",
}

const origLinkHeaderPrefix = "X-SES-Forwarder-Original: s3://"

func (hb *headerBuffer) WriteUpdatedHeaders(input *updateHeadersInput) error {
	hb.writeFromAndReplyTo(input.headers, input.senderAddress)

	for _, header := range keepHeaders {
		if values, ok := input.headers[header]; ok {
			hb.writeHeader(header, values)
		}
	}
	hb.write(origLinkHeaderPrefix + input.msgPath + "\r\n\r\n")

	if hb.err != nil {
		return fmt.Errorf("error updating email headers: %s", hb.err)
	}
	return nil
}

func (hb *headerBuffer) writeFromAndReplyTo(
	headers mail.Header, sender string,
) {
	origFrom := headers.Get("From")
	replyTo := headers.Get("Reply-To")
	var newFrom string

	newFrom, hb.err = newFromAddress(origFrom, sender)
	if hb.err != nil {
		return
	}

	hb.writeHeader("From", []string{newFrom})
	if replyTo == "" {
		replyTo = origFrom
	}
	hb.writeHeader("Reply-To", []string{replyTo})
}

func newFromAddress(origFrom, newFrom string) (result string, err error) {
	var addr *mail.Address

	if addr, err = mail.ParseAddress(origFrom); err != nil {
		err = fmt.Errorf("couldn't parse From address %s: %s", origFrom, err)
	} else {
		if addr.Name != "" {
			addr.Name += " - "
		}

		// Gmail parses the first address out of the From header for the purpose
		// of checking SPF and DMARC status. It will ignore a later address
		// appearing within angle brackets, which should be treated as the
		// actual From address. Replacing the "@" with " at " in the original
		// address avoids this problem, confirmed by Gmail's "Show Original"
		// message view.
		addrReplaced := strings.Replace(addr.Address, "@", " at ", 1)
		result = addr.Name + addrReplaced + " <" + newFrom + ">"
	}
	return
}

func (hb *headerBuffer) writeHeader(name string, values []string) {
	// Note that according to RFC 2045 Section 4, the header must be verbatim:
	// "MIME-Version: 1.0".
	// - https://www.rfc-editor.org/rfc/rfc2045#section-4
	//
	// Technically the headers should be case insensitive; see
	// https://stackoverflow.com/a/6143644, which explains RFC 5322. In fact,
	// the Go standard library net/textproto package parses all email headers
	// using CanonicalMIMEHeaderKey:
	// - https://pkg.go.dev/net/textproto#CanonicalMIMEHeaderKey
	//
	// However, it's been reported that some mail servers choke on messages
	// that don't use "MIME-Version" exactly. For this reason, we make sure to
	// always emit it.
	if name == "Mime-Version" {
		name = "MIME-Version"
	}

	for _, value := range values {
		hb.write(name + ": " + value + "\r\n")
	}
}

func (hb *headerBuffer) write(s string) {
	if hb.err == nil {
		_, hb.err = hb.buf.Write([]byte(s))
	}
}
