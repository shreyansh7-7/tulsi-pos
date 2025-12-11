package awsclient

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var S3 *s3.Client
var AWSRegion string
var InvoiceBucket string

func InitAWS() {
	AWSRegion = os.Getenv("AWS_REGION")
	InvoiceBucket = os.Getenv("S3_BUCKET_INVOICES")

	if AWSRegion == "" || InvoiceBucket == "" {
		log.Println("⚠️ AWS_REGION or S3_BUCKET_INVOICES not set, S3 disabled")
		return
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(AWSRegion))
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	S3 = s3.NewFromConfig(cfg)
	log.Println("✅ AWS S3 client initialized")
}
