package handler

import (
	"fmt"
	"io"
	"net/mail"
)

type headerBuffer struct {
	buf io.Writer
	err error
}

type updateHeadersInput struct {
	headers       mail.Header
	senderAddress string
	bucketName    string
	msgKey        string
}

var keepHeaders = map[string]bool{
	"To":           true,
	"Cc":           true,
	"Bcc":          true,
	"Subject":      true,
	"Reply-To":     true,
	"Content-Type": true,
	"MIME-Version": true,
	"Mime-Version": true,
}

func (hb *headerBuffer) WriteUpdatedHeaders(input *updateHeadersInput) error {
	hb.writeFromHeader(input.headers, input.senderAddress)

	for header, values := range input.headers {
		if header == "Mime-Version" {
			header = "MIME-Version"
		}
		if keepHeaders[header] {
			hb.writeHeader(header, values)
		}
	}
	hb.writeFinalSesForwarderOrigLinkHeader(input.bucketName, input.msgKey)

	if hb.err != nil {
		return fmt.Errorf("error while updating email headers: %s", hb.err)
	}
	return nil
}

func (hb *headerBuffer) writeFromHeader(headers mail.Header, sender string) {
	origFrom := headers.Get("From")
	newFrom := hb.newFromAddress(origFrom, sender)
	if hb.err != nil {
		return
	}

	hb.writeHeader("From", []string{newFrom})
	if headers.Get("Reply-To") == "" {
		hb.writeHeader("Reply-To", []string{origFrom})
	}
}

func (hb *headerBuffer) newFromAddress(origFrom, newFrom string) string {
	fromAddr, err := mail.ParseAddress(origFrom)
	if err != nil {
		hb.err = fmt.Errorf("couldn't parse From address %s: %s", origFrom, err)
		return ""
	}

	const newFromFmt = "%s at %s <%s>"
	return fmt.Sprintf(newFromFmt, fromAddr.Name, fromAddr.Address, newFrom)
}

const crlf = "\r\n"

func (hb *headerBuffer) writeFinalSesForwarderOrigLinkHeader(
	bucketName, msgKey string,
) {
	origLink := "s3://" + bucketName + "/" + msgKey
	hb.writeHeader("X-SES-Forwarder-Original", []string{origLink})
	hb.write(crlf + crlf)
}

func (hb *headerBuffer) writeHeader(name string, values []string) {
	for _, value := range values {
		hb.write(name)
		hb.write(": ")
		hb.write(value)
		hb.write(crlf)
	}
}

func (hb *headerBuffer) write(s string) {
	if hb.err != nil {
		_, hb.err = hb.buf.Write([]byte(s))
	}
}
