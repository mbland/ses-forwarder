//go:build small_tests || all_tests

package handler

import (
	"testing"

	"gotest.tools/assert"
)

func TestUndefinedEnvVarsErrorFormat(t *testing.T) {
	assert.ErrorContains(
		t,
		&UndefinedEnvVarsError{UndefinedVars: []string{"FOO", "BAR", "BAZ"}},
		"undefined environment variables: FOO, BAR, BAZ",
	)
}

func TestReportUndefinedEnviromentVariables(t *testing.T) {
	_, err := GetOptions(func(string) string { return "" })

	assert.DeepEqual(
		t,
		err,
		&UndefinedEnvVarsError{
			UndefinedVars: []string{
				"BUCKET_NAME",
				"INCOMING_PREFIX",
				"FORWARDING_ADDRESS",
				"CONFIGURATION_SET",
			},
		},
	)
}

func TestAllRequiredEnvironmentVariablesDefined(t *testing.T) {
	env := map[string]string{
		"BUCKET_NAME":        "my-bucket",
		"INCOMING_PREFIX":    "inbox",
		"FORWARDING_ADDRESS": "me@bar.com",
		"CONFIGURATION_SET":  "config-set",
	}
	opts, err := GetOptions(func(varname string) string {
		return env[varname]
	})

	assert.NilError(t, err)
	assert.DeepEqual(
		t,
		opts,
		&Options{
			BucketName:        "my-bucket",
			IncomingPrefix:    "inbox",
			ForwardingAddress: "me@bar.com",
			ConfigurationSet:  "config-set",
		},
	)
}
