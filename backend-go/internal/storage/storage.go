// Package storage is the Cloudflare R2 client for content we host ourselves —
// own scanlation pages first. R2 speaks the S3
// API, so this is aws-sdk-go-v2 pointed at the account endpoint. The feature
// is optional: a nil *Client means "not configured" and callers degrade to
// local disk or a clear error.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"animekage/backend/internal/config"
)

type Client struct {
	s3        *s3.Client
	bucket    string
	publicURL string // base the world fetches objects from (custom domain / r2.dev)
}

// New returns a configured client, or nil when the env doesn't configure R2 —
// callers treat nil as "feature off", the same contract as ANTHROPIC_API_KEY.
// R2PublicURL is deliberately NOT required here. Serving objects to the world
// needs it; accepting uploads does not, and requiring it meant a bucket
// configured purely for ingest looked like "feature off".
// Put is the one operation that cannot work without it and says so.
func New(cfg *config.Config) (*Client, error) {
	if cfg.R2AccessKey == "" || cfg.R2SecretKey == "" || cfg.R2Bucket == "" {
		return nil, nil
	}
	endpoint := cfg.R2Endpoint
	if endpoint == "" {
		if cfg.R2AccountID == "" {
			return nil, fmt.Errorf("storage: R2_ACCOUNT_ID or R2_ENDPOINT is required")
		}
		endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID)
	}

	awsCfg, err := awscfg.LoadDefaultConfig(context.Background(),
		awscfg.WithRegion("auto"),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.R2AccessKey, cfg.R2SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}

	c := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		// R2 prefers path-style addressing (bucket in the path, not the host).
		o.UsePathStyle = true
	})

	return &Client{
		s3:        c,
		bucket:    cfg.R2Bucket,
		publicURL: strings.TrimRight(cfg.R2PublicURL, "/"),
	}, nil
}

// PresignPut mints a URL that lets the *browser* PUT one object straight into
// the bucket, without the bytes ever passing through this server.
//
// How it works: the SDK builds the exact request it would have sent — method,
// host, path, and the headers that matter (here Content-Type) — signs that
// canonical form with the secret key using AWS SigV4, and puts the signature,
// the access key id, a timestamp and an expiry into the query string instead of
// an Authorization header. The result is a plain URL that encodes one specific
// permission: "PUT this exact key, with this exact content type, until this
// moment". R2 recomputes the signature from the request it receives and compares.
//
// Three consequences worth knowing:
//
//   - No credential ever reaches the browser. The signature is derived from the
//     secret but does not contain it, and it cannot be replayed against a
//     different key, method or content type — change any signed element and the
//     recomputed signature no longer matches.
//   - Nothing is contacted to mint it. Signing is pure computation, so this call
//     makes no network request and cannot fail for connectivity reasons. The URL
//     exists whether or not the bucket does.
//   - It expires. ttl is baked into the signature, so a leaked URL is a
//     time-boxed capability rather than an open door. An hour is plenty for a
//     multi-gigabyte upload on a domestic connection and short enough to matter.
//
// This is why it is the right shape for our problem: the upload bypasses both
// this server's disk and Cloudflare's 100 MiB proxy body limit, because the
// browser is talking to r2.cloudflarestorage.com directly and we are only
// handing out permission.
func (c *Client) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, error) {
	key = strings.TrimLeft(key, "/")
	req, err := s3.NewPresignClient(c.s3).PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("storage: presign put %s: %w", key, err)
	}
	return req.URL, nil
}

// PresignGet mints a time-limited URL for *reading* one object.
//
// The read twin of PresignPut, and it is what lets the video stay in the bucket:
// ffmpeg takes this as an input URL, and the browser can use it as a <video> src
// so preview bytes never pass through this server at all.
func (c *Client) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	key = strings.TrimLeft(key, "/")
	in := &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}
	// Override the stored Content-Type on the way out.
	//
	// Uploads are signed as application/octet-stream, because the presigned PUT
	// has to be signed before anyone knows what the browser will send — so that
	// is what R2 stores. A <video> element handed octet-stream refuses to play
	// it. S3's response-content-type parameter rewrites the header per request,
	// which fixes objects already in the bucket without re-uploading them.
	if ct := videoContentType(key); ct != "" {
		in.ResponseContentType = aws.String(ct)
	}
	req, err := s3.NewPresignClient(c.s3).PresignGetObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("storage: presign get %s: %w", key, err)
	}
	return req.URL, nil
}

// videoContentType maps an object key's extension to something a browser will
// actually try to decode. Matroska is included for completeness, but note no
// browser plays it natively — see the preview note in handler/releases.go.
func videoContentType(key string) string {
	switch strings.ToLower(path.Ext(key)) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	}
	return ""
}

// Size returns an object's length, or ErrNotFound when it isn't there.
//
// The completeness check for an upload: a browser that dies mid-PUT leaves a
// short object, and R2 will happily serve it. Comparing against the size the
// client declared up front is what stops a truncated video becoming a release.
func (c *Client) Size(ctx context.Context, key string) (int64, error) {
	key = strings.TrimLeft(key, "/")
	out, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nf *types.NotFound
		if errors.As(err, &nf) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("storage: head %s: %w", key, err)
	}
	return aws.ToInt64(out.ContentLength), nil
}

// Get opens an object for reading. The caller closes it.
func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	key = strings.TrimLeft(key, "/")
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nf *types.NoSuchKey
		if errors.As(err, &nf) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage: get %s: %w", key, err)
	}
	return out.Body, nil
}

// ErrNotFound is returned when an object does not exist, so callers can tell
// "never uploaded" from "R2 is unreachable" — one is a client error, the other
// is a 500.
var ErrNotFound = errors.New("storage: object not found")

// PutStream uploads one object without caring about its public URL.
//
// Split out from Put because backups have no business needing R2_PUBLIC_URL:
// nothing ever fetches them over HTTP, they are pulled back with the S3 API on
// the day something has gone wrong.
func (c *Client) PutStream(ctx context.Context, key, contentType string, body io.Reader) error {
	key = strings.TrimLeft(key, "/")
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	return nil
}

// List returns the keys under a prefix, oldest-sorted by name. Used by backup
// retention, where the key carries the timestamp.
func (c *Client) List(ctx context.Context, prefix string) ([]string, error) {
	out, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: list %s: %w", prefix, err)
	}
	keys := make([]string, 0, len(out.Contents))
	for _, o := range out.Contents {
		keys = append(keys, aws.ToString(o.Key))
	}
	sort.Strings(keys)
	return keys, nil
}

// Put uploads one object and returns its public URL.
func (c *Client) Put(ctx context.Context, key, contentType string, body io.Reader) (string, error) {
	if c.publicURL == "" {
		return "", fmt.Errorf("storage: R2_PUBLIC_URL must be set to publish %s", key)
	}
	if err := c.PutStream(ctx, key, contentType, body); err != nil {
		return "", err
	}
	return c.publicURL + "/" + strings.TrimLeft(key, "/"), nil
}

// Delete removes one object; missing objects are not an error (S3 semantics).
func (c *Client) Delete(ctx context.Context, key string) error {
	key = strings.TrimLeft(key, "/")
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	return nil
}
