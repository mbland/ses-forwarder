package handler

import "strings"

type Options struct {
	BucketName        string
	IncomingPrefix    string
	EmailDomainName   string
	SenderAddress     string
	ForwardingAddress string
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
	env.assign(&opts.EmailDomainName, "EMAIL_DOMAIN_NAME")
	env.assign(&opts.SenderAddress, "SENDER_ADDRESS")
	env.assign(&opts.ForwardingAddress, "FORWARDING_ADDRESS")

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
