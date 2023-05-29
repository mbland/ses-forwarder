# SES-forwarder

Amazon Simple Email Service Receipt Rule email forwarding system.

Source: <https://github.com/mbland/ses-forwarder>

[![License](https://img.shields.io/github/license/mbland/ses-forwarder.svg)](https://github.com/mbland/ses-forwarder/blob/main/LICENSE.txt)
[![CI status](https://github.com/mbland/ses-forwarder/actions/workflows/run-tests.yaml/badge.svg)](https://github.com/mbland/ses-forwarder/actions/workflows/run-tests.yaml?branch=main)
[![Coverage Status](https://coveralls.io/repos/github/mbland/ses-forwarder/badge.svg?branch=main)](https://coveralls.io/github/mbland/ses-forwarder?branch=main)

_(Try force reloading the page to get the latest badges if this is a return
visit. [The browser cache may hide the latest
results](https://stackoverflow.com/a/37894321).)_

Implemented in [Go][] using the following [Amazon Web Services][]:

- [Lambda][]
- [Simple Email Service][]

Uses [CloudFormation][] and the [AWS Serverless Application Model (SAM)][] for
deploying the Lambda function, managing permissions, and other configuration
parameters.

Based on:

- [AWS Messaging & Targeting Blog: Forward Incoming Email to an External Destination][]
- [mbland/elistman][]

See also:

- [arithmetric/aws-lambda-ses-forwarder][]

[Go]: https://go.dev/
[Amazon Web Services]: https://aws.amazon.com
[Lambda]: https://aws.amazon.com/lambda/
[Simple Email Service]: https://aws.amazon.com/ses/
[CloudFormation]: https://aws.amazon.com/cloudformation/
[AWS Serverless Application Model (SAM)]: https://aws.amazon.com/serverless/sam/
[AWS Messaging & Targeting Blog: Forward Incoming Email to an External Destination]: https://aws.amazon.com/blogs/messaging-and-targeting/forward-incoming-email-to-an-external-destination/
[mbland/elistman]: https://github.com/mbland/elistman
[arithmetric/aws-lambda-ses-forwarder]: https://github.com/arithmetric/aws-lambda-ses-forwarder
