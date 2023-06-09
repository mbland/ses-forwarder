package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/mbland/ses-forwarder/handler"
)

func buildHandler() (*handler.Handler, error) {
	if cfg, err := config.LoadDefaultConfig(context.Background()); err != nil {
		return nil, err
	} else if opts, err := handler.GetOptions(os.Getenv); err != nil {
		return nil, err
	} else {
		return &handler.Handler{
			S3:      s3.NewFromConfig(cfg),
			Ses:     ses.NewFromConfig(cfg),
			SesV2:   sesv2.NewFromConfig(cfg),
			Options: opts,
			Log:     log.Default(),
		}, nil
	}
}

func main() {
	// Disable standard logger flags. The CloudWatch logs show that the Lambda
	// runtime already adds a timestamp at the beginning of every log line
	// emitted by the function.
	log.SetFlags(0)

	if h, err := buildHandler(); err != nil {
		log.Fatalf("Failed to initialize process: %s", err.Error())
	} else {
		lambda.Start(h.HandleEvent)
	}
}
