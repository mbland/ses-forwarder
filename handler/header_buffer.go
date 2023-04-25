package handler

import (
	"fmt"
	"io"
	"net/mail"
)

type headerBuffer struct {
	buf           io.Writer
	headers       mail.Header
	senderAddress string
	bucketName    string
	msgKey        string
	err           error
}

const crlf = "\r\n"

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

func (hb *headerBuffer) WriteUpdatedHeaders() error {
	hb.WriteFromHeader()

	for header, values := range hb.headers {
		if keepHeaders[header] {
			hb.WriteHeader(header, values)
		}
	}
	hb.WriteFinalSesForwarderOrigLinkHeader()

	if hb.err != nil {
		return fmt.Errorf("error while updating email headers: %s", hb.err)
	}
	return nil
}

func (hb *headerBuffer) WriteFromHeader() {
	origFrom := hb.headers.Get("From")
	newFrom := hb.newFromAddress(origFrom)
	if hb.err != nil {
		return
	}

	hb.WriteHeader("From", []string{newFrom})
	if hb.headers.Get("Reply-To") == "" {
		hb.WriteHeader("Reply-To", []string{origFrom})
	}
}

func (hb *headerBuffer) newFromAddress(origFrom string) string {
	fromAddr, err := mail.ParseAddress(origFrom)
	if err != nil {
		hb.err = fmt.Errorf("couldn't parse From address %s: %s", origFrom, err)
		return ""
	}

	const newFromFmt = "%s at %s <%s>"
	return fmt.Sprintf(
		newFromFmt, fromAddr.Name, fromAddr.Address, hb.senderAddress,
	)
}

func (hb *headerBuffer) WriteFinalSesForwarderOrigLinkHeader() {
	origLink := "s3://" + hb.bucketName + "/" + hb.msgKey
	hb.WriteHeader("X-SES-Forwarder-Original", []string{origLink})
	hb.Write(crlf + crlf)
}

func (hb *headerBuffer) WriteHeader(name string, values []string) {
	if name == "Mime-Version" {
		name = "MIME-Version"
	}
	for _, value := range values {
		hb.Write(name)
		hb.Write(": ")
		hb.Write(value)
		hb.Write(crlf)
	}
}

func (hb *headerBuffer) Write(s string) {
	if hb.err != nil {
		_, hb.err = hb.buf.Write([]byte(s))
	}
}
