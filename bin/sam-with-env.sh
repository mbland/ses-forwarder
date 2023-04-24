#!/bin/bash

ENV_FILE="$1"
shift

if [[ -z "$ENV_FILE" ]]; then
  printf 'Usage: %s [deployment environment variables file] [sam args...]\n' \
    "$0" >&2
  exit 1
elif [[ "$#" -eq 0 ]]; then
  printf "No arguments for 'sam' command given.\n" >&2
  exit 1
fi
. "$ENV_FILE" || exit 1

# Process RecipientConditions first, since it can take several forms.
printf "${RECIPIENT_CONDITIONS:?}" >/dev/null
RECIPIENT_CONDITIONS="${RECIPIENT_CONDITIONS//$'\r'/}"

PARAMETER_OVERRIDES=(
  "BucketName=${BUCKET_NAME:?}"
  "IncomingPrefix=${INCOMING_PREFIX:?}"
  "EmailDomainName=${EMAIL_DOMAIN_NAME:?}"
  "RecipientConditions=${RECIPIENT_CONDITIONS//$'\n'/,}"
  "SenderAddress=${SENDER_ADDRESS:?}"
  "ForwardingAddress=${FORWARDING_ADDRESS:?}"
  "ReceiptRuleSetName=${RECEIPT_RULE_SET_NAME:?}"
)

export SAM_CLI_TELEMETRY=0
exec sam "${@}" --parameter-overrides "${PARAMETER_OVERRIDES[*]}"
