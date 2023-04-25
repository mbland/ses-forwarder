//go:build small_tests || all_tests

package handler

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gotest.tools/assert"
)

func TestNewFromAddress(t *testing.T) {
	h := Handler{Options: &Options{SenderAddress: "ses-forwarder@foo.com"}}

	t.Run("Succeeds", func(t *testing.T) {
		newFrom, err := h.newFromAddress("Mike Bland <mbland@acm.org>")

		assert.NilError(t, err)
		expected := "Mike Bland at mbland@acm.org <ses-forwarder@foo.com>"
		assert.Equal(t, expected, newFrom)
	})

	t.Run("FailsIfOriginalFromMalformed", func(t *testing.T) {
		newFrom, err := h.newFromAddress("Mike Bland mbland@acm.org")

		assert.Equal(t, "", newFrom)
		assert.ErrorContains(t, err, "couldn't extract From address: ")
	})
}

type TestS3 struct {
	input     *s3.GetObjectInput
	output    *s3.GetObjectOutput
	returnErr error
}

func (s3 *TestS3) GetObject(
	ctx context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options),
) (*s3.GetObjectOutput, error) {
	s3.input = input
	return s3.output, s3.returnErr
}

type ErrReader struct {
	err error
}

func (r *ErrReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestGetOriginalMessage(t *testing.T) {
	setup := func() (*TestS3, *Handler, context.Context) {
		testS3 := &TestS3{output: &s3.GetObjectOutput{}}
		opts := &Options{BucketName: "mail.foo.com"}
		ctx := context.Background()
		return testS3, &Handler{S3: testS3, Options: opts}, ctx
	}

	t.Run("Succeeds", func(t *testing.T) {
		testS3, h, ctx := setup()
		testS3.output.Body = io.NopCloser(strings.NewReader("Hello, world!"))
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

		assert.Equal(t, len(msg), 0)
		expected := "failed to get original message: S3 test error"
		assert.ErrorContains(t, err, expected)
	})

	t.Run("ErrorsIfReadingBodyFails", func(t *testing.T) {
		testS3, h, ctx := setup()
		r := &ErrReader{errors.New("test read error")}
		testS3.output.Body = io.NopCloser(r)

		msg, err := h.getOriginalMessage(ctx, "prefix/msgId")

		assert.Equal(t, len(msg), 0)
		expected := "failed to get original message: test read error"
		assert.ErrorContains(t, err, expected)
	})
}
