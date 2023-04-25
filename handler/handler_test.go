//go:build small_tests || all_tests

package handler

import (
	"testing"

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
