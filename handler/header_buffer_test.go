//go:build small_tests || all_tests

package handler

import (
	"testing"

	"gotest.tools/assert"
)

func TestNewFromAddress(t *testing.T) {
	hb := &headerBuffer{senderAddress: "ses-forwarder@foo.com"}

	t.Run("Succeeds", func(t *testing.T) {
		newFrom := hb.newFromAddress("Mike Bland <mbland@acm.org>")

		assert.NilError(t, hb.err)
		expected := "Mike Bland at mbland@acm.org <ses-forwarder@foo.com>"
		assert.Equal(t, expected, newFrom)
	})

	t.Run("FailsIfOriginalFromMalformed", func(t *testing.T) {
		const addr = "Mike Bland mbland@acm.org"

		newFrom := hb.newFromAddress(addr)

		assert.Equal(t, "", newFrom)
		assert.ErrorContains(t, hb.err, "couldn't parse From address "+addr)
	})
}
