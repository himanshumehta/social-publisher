package infra

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/himanshumehta/social-publisher/internal/config"
)

type S3Client struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
	endpoint  string // scheme://host, used to build path-style public URLs
}

type CompletedPart struct {
	PartNumber int
	ETag       string
}

func NewS3Client(cfg config.S3Config) *S3Client {
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	endpoint := fmt.Sprintf("%s://%s", scheme, cfg.Endpoint)

	client := s3.New(s3.Options{
		Region: cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKey, cfg.SecretKey, "",
		),
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: true,
	})

	presigner := s3.NewPresignClient(client)

	return &S3Client{
		client:    client,
		presigner: presigner,
		bucket:    cfg.Bucket,
		endpoint:  endpoint,
	}
}

func (s *S3Client) CreateMultipartUpload(ctx context.Context, key, contentType string) (string, error) {
	out, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("create multipart upload: %w", err)
	}
	return *out.UploadId, nil
}

func (s *S3Client) PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int, expiry time.Duration) (string, error) {
	out, err := s.presigner.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(int32(partNumber)),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("presign upload part %d: %w", partNumber, err)
	}
	return out.URL, nil
}

func (s *S3Client) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error {
	completedParts := make([]types.CompletedPart, len(parts))
	for i, p := range parts {
		completedParts[i] = types.CompletedPart{
			PartNumber: aws.Int32(int32(p.PartNumber)),
			ETag:       aws.String(p.ETag),
		}
	}

	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("complete multipart upload: %w", err)
	}
	return nil
}

// ListParts returns the completed parts (with ETags) for an in-progress multipart upload.
func (s *S3Client) ListParts(ctx context.Context, key, uploadID string) ([]CompletedPart, error) {
	var parts []CompletedPart
	var partMarker *string
	for {
		out, err := s.client.ListParts(ctx, &s3.ListPartsInput{
			Bucket:           aws.String(s.bucket),
			Key:              aws.String(key),
			UploadId:         aws.String(uploadID),
			PartNumberMarker: partMarker,
		})
		if err != nil {
			return nil, fmt.Errorf("list parts: %w", err)
		}
		for _, p := range out.Parts {
			parts = append(parts, CompletedPart{
				PartNumber: int(aws.ToInt32(p.PartNumber)),
				ETag:       aws.ToString(p.ETag),
			})
		}
		if !aws.ToBool(out.IsTruncated) {
			break
		}
		partMarker = out.NextPartNumberMarker
	}
	return parts, nil
}

func (s *S3Client) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	_, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		return fmt.Errorf("abort multipart upload: %w", err)
	}
	return nil
}

// Bucket returns the bucket name for this client.
func (s *S3Client) Bucket() string {
	return s.bucket
}

// CopyObject performs a server-side copy from srcBucket/srcKey to dstBucket/dstKey.
// Works for files up to 5GB on DO Spaces.
func (s *S3Client) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	copySource := srcBucket + "/" + srcKey
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(dstBucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(copySource),
	})
	if err != nil {
		return fmt.Errorf("copy object %s → %s/%s: %w", copySource, dstBucket, dstKey, err)
	}
	return nil
}

// CopyObjectMultipart performs a multipart server-side copy for files larger than 5GB.
func (s *S3Client) CopyObjectMultipart(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string, sizeBytes int64) error {
	const partSize int64 = 500 * 1024 * 1024 // 500MB parts

	createOut, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(dstBucket),
		Key:    aws.String(dstKey),
	})
	if err != nil {
		return fmt.Errorf("create multipart copy: %w", err)
	}
	uploadID := *createOut.UploadId

	copySource := srcBucket + "/" + srcKey
	var completedParts []types.CompletedPart
	var partNum int32 = 1

	for offset := int64(0); offset < sizeBytes; offset += partSize {
		end := offset + partSize - 1
		if end >= sizeBytes {
			end = sizeBytes - 1
		}
		byteRange := fmt.Sprintf("bytes=%d-%d", offset, end)

		partOut, err := s.client.UploadPartCopy(ctx, &s3.UploadPartCopyInput{
			Bucket:          aws.String(dstBucket),
			Key:             aws.String(dstKey),
			CopySource:      aws.String(copySource),
			UploadId:        aws.String(uploadID),
			PartNumber:      aws.Int32(partNum),
			CopySourceRange: aws.String(byteRange),
		})
		if err != nil {
			// Abort on failure
			_, _ = s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
				Bucket:   aws.String(dstBucket),
				Key:      aws.String(dstKey),
				UploadId: aws.String(uploadID),
			})
			return fmt.Errorf("upload part copy %d: %w", partNum, err)
		}

		completedParts = append(completedParts, types.CompletedPart{
			PartNumber: aws.Int32(partNum),
			ETag:       partOut.CopyPartResult.ETag,
		})
		partNum++
	}

	_, err = s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(dstBucket),
		Key:      aws.String(dstKey),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("complete multipart copy: %w", err)
	}
	return nil
}

// PresignPutObject generates a presigned PUT URL for uploading a single small object.
func (s *S3Client) PresignPutObject(ctx context.Context, key, contentType string, expiry time.Duration) (string, error) {
	out, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("presign put object %s: %w", key, err)
	}
	return out.URL, nil
}

// PresignGetObject generates a presigned GET URL for an object.
func (s *S3Client) PresignGetObject(ctx context.Context, key string, expiry time.Duration) (string, error) {
	out, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("presign get object %s: %w", key, err)
	}
	return out.URL, nil
}

// DeleteObject deletes an object from this client's bucket.
func (s *S3Client) DeleteObject(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete object %s: %w", key, err)
	}
	return nil
}

// GetObjectStream returns a streaming reader for an S3 object along with its size.
// Caller is responsible for closing the returned ReadCloser.
func (s *S3Client) GetObjectStream(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("get object stream %s: %w", key, err)
	}
	size := int64(0)
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return out.Body, size, nil
}

// GetObject downloads an object from S3 and returns its content as bytes.
func (s *S3Client) GetObject(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", key, err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("read object %s: %w", key, err)
	}
	return data, nil
}

// UploadObject uploads data to S3 and returns the public URL.
func (s *S3Client) UploadObject(ctx context.Context, key, contentType string, data []byte) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
		Body:        bytes.NewReader(data),
		ACL:         types.ObjectCannedACLPublicRead, // Make thumbnail publicly accessible
	})
	if err != nil {
		return "", fmt.Errorf("upload object %s: %w", key, err)
	}

	// Construct the public path-style URL: {endpoint}/{bucket}/{key}
	url := fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, key)
	return url, nil
}
