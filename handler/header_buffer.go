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
	"Reply-To",
	"To",
	"Cc",
	"Bcc",
	"Subject",
	"MIME-Version",
	"Mime-Version",
	"Content-Type",
}

const origLinkHeaderPrefix = "X-SES-Forwarder-Original: s3://"

func (hb *headerBuffer) WriteUpdatedHeaders(input *updateHeadersInput) error {
	hb.writeFromHeader(input.headers, input.senderAddress)

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

func (hb *headerBuffer) writeFromHeader(headers mail.Header, sender string) {
	origFrom := headers.Get("From")
	var newFrom string

	newFrom, hb.err = newFromAddress(origFrom, sender)
	if hb.err != nil {
		return
	}

	hb.writeHeader("From", []string{newFrom})
	if headers.Get("Reply-To") == "" {
		hb.writeHeader("Reply-To", []string{origFrom})
	}
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
