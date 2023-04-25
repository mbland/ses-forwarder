package handler

import "strings"

type Options struct {
	BucketName        string
	IncomingPrefix    string
	SenderAddress     string
	ForwardingAddress string
	ConfigurationSet  string
}

type UndefinedEnvVarsError struct {
	UndefinedVars []string
}

func (e *UndefinedEnvVarsError) Error() string {
	return "undefined environment variables: " +
		strings.Join(e.UndefinedVars, ", ")
}

func GetOptions(getenv func(string) string) (*Options, error) {
	env := environment{getenv: getenv}
	return env.options()
}

type environment struct {
	getenv        func(string) string
	undefinedVars []string
}

func (env *environment) options() (*Options, error) {
	opts := Options{}
	env.assign(&opts.BucketName, "BUCKET_NAME")
	env.assign(&opts.IncomingPrefix, "INCOMING_PREFIX")
	env.assign(&opts.SenderAddress, "SENDER_ADDRESS")
	env.assign(&opts.ForwardingAddress, "FORWARDING_ADDRESS")
	env.assign(&opts.ConfigurationSet, "CONFIGURATION_SET")

	if len(env.undefinedVars) != 0 {
		return nil, &UndefinedEnvVarsError{UndefinedVars: env.undefinedVars}
	}
	return &opts, nil
}

func (env *environment) assign(opt *string, varname string) {
	if value := env.getenv(varname); value == "" {
		env.undefinedVars = append(env.undefinedVars, varname)
	} else {
		*opt = value
	}
}
