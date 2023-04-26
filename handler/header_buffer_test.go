//go:build small_tests || all_tests

package handler

import (
	"testing"

	"gotest.tools/assert"
)

func TestNewFromAddress(t *testing.T) {
	senderAddress := "ses-forwarder@foo.com"

	t.Run("Succeeds", func(t *testing.T) {
		newFrom, err := newFromAddress(
			"Mike Bland <mbland@acm.org>", senderAddress,
		)

		assert.NilError(t, err)
		expected := "Mike Bland at mbland@acm.org <ses-forwarder@foo.com>"
		assert.Equal(t, expected, newFrom)
	})

	t.Run("FailsIfOriginalFromMalformed", func(t *testing.T) {
		const addr = "Mike Bland mbland@acm.org"

		newFrom, err := newFromAddress(addr, senderAddress)

		assert.Equal(t, "", newFrom)
		assert.ErrorContains(t, err, "couldn't parse From address "+addr)
	})
}
