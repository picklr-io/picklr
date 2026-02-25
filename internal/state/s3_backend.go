package state

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/ir"
)

// s3Backend implements Backend for AWS S3 + optional DynamoDB locking.
type s3Backend struct {
	bucket        string
	key           string
	region        string
	dynamoDBTable string
	encrypt       bool
	profile       string

	evaluator *eval.Evaluator
	s3Client  *s3.Client
	dbClient  *dynamodb.Client
	lockID    string
}

func newS3Backend(config map[string]string, evaluator *eval.Evaluator) (Backend, error) {
	bucket := config["bucket"]
	if bucket == "" {
		return nil, fmt.Errorf("s3 backend requires 'bucket' configuration")
	}

	key := config["key"]
	if key == "" {
		key = "picklr/state.pkl"
	}

	region := config["region"]
	if region == "" {
		region = "us-east-1"
	}

	b := &s3Backend{
		bucket:        bucket,
		key:           key,
		region:        region,
		dynamoDBTable: config["dynamodb_table"],
		encrypt:       config["encrypt"] == "true",
		profile:       config["profile"],
		evaluator:     evaluator,
	}

	if err := b.initClients(); err != nil {
		return nil, fmt.Errorf("failed to initialize S3 backend: %w", err)
	}

	return b, nil
}

func (b *s3Backend) initClients() error {
	ctx := context.Background()

	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(b.region))
	if b.profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(b.profile))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("unable to load AWS config: %w", err)
	}

	b.s3Client = s3.NewFromConfig(cfg)

	if b.dynamoDBTable != "" {
		b.dbClient = dynamodb.NewFromConfig(cfg)
	}

	return nil
}

func (b *s3Backend) Read(ctx context.Context) (*ir.State, error) {
	result, err := b.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.key),
	})
	if err != nil {
		// If the object doesn't exist, return empty state
		var nsk *s3types.NoSuchKey
		if errors.As(err, &nsk) {
			return &ir.State{Version: 1, Serial: 0}, nil
		}
		// Also handle 404 via the error message for S3 API variations
		if strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "404") {
			return &ir.State{Version: 1, Serial: 0}, nil
		}
		return nil, fmt.Errorf("failed to read state from s3://%s/%s: %w", b.bucket, b.key, err)
	}
	defer result.Body.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(result.Body); err != nil {
		return nil, fmt.Errorf("failed to read S3 object body: %w", err)
	}
	content := buf.Bytes()

	// Handle encryption
	if IsEncrypted(content) {
		decrypted, err := DecryptState(content)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt remote state: %w", err)
		}
		content = decrypted
	}

	// Write to a temporary file so the PKL evaluator can parse it
	tmpFile, err := os.CreateTemp("", "picklr-state-*.pkl")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("failed to write temp state file: %w", err)
	}
	tmpFile.Close()

	state, err := b.evaluator.LoadState(ctx, tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to parse remote state: %w", err)
	}

	return state, nil
}

func (b *s3Backend) Write(ctx context.Context, state *ir.State) error {
	content := SerializeState(state)

	// Encrypt if configured
	data := []byte(content)
	encrypted, err := EncryptState(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt state: %w", err)
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.key),
		Body:   bytes.NewReader(encrypted),
	}
	if b.encrypt {
		input.ServerSideEncryption = s3types.ServerSideEncryptionAes256
	}

	if _, err := b.s3Client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("failed to write state to s3://%s/%s: %w", b.bucket, b.key, err)
	}

	return nil
}

func (b *s3Backend) Lock() error {
	if b.dynamoDBTable == "" {
		return nil // No locking without DynamoDB
	}

	b.lockID = fmt.Sprintf("picklr-%d-%d", os.Getpid(), time.Now().UnixNano())

	_, err := b.dbClient.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(b.dynamoDBTable),
		Item: map[string]dbtypes.AttributeValue{
			"LockID":  &dbtypes.AttributeValueMemberS{Value: b.key},
			"Info":    &dbtypes.AttributeValueMemberS{Value: b.lockID},
			"Created": &dbtypes.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		},
		ConditionExpression: aws.String("attribute_not_exists(LockID)"),
	})
	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return fmt.Errorf("state is locked by another process. If this is an error, "+
				"manually delete the lock item with LockID=%q from DynamoDB table %q", b.key, b.dynamoDBTable)
		}
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	return nil
}

func (b *s3Backend) Unlock() error {
	if b.dynamoDBTable == "" {
		return nil
	}

	_, err := b.dbClient.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(b.dynamoDBTable),
		Key: map[string]dbtypes.AttributeValue{
			"LockID": &dbtypes.AttributeValueMemberS{Value: b.key},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	return nil
}
