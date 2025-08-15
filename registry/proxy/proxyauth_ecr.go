package proxy

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	awsCredentials "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/sirupsen/logrus"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/internal/client/auth"
)

var ecrURLPattern = regexp.MustCompile(`^(\d+)\.dkr\.ecr\.([^.]+)\.amazonaws\.com$`)

type ecrCredentials struct {
	m          sync.Mutex
	client     *ecr.ECR
	registryID string
	lifetime   *time.Duration
	username   string
	password   string
	expiry     time.Time
}

// Basic implements the auth.CredentialStore interface
func (c *ecrCredentials) Basic(url *url.URL) (string, string) {
	c.m.Lock()
	defer c.m.Unlock()

	now := time.Now()
	if c.username != "" && c.password != "" && (c.lifetime == nil || now.Before(c.expiry)) {
		return c.username, c.password
	}

	// Get authorization token from ECR
	input := &ecr.GetAuthorizationTokenInput{}
	if c.registryID != "" {
		input.RegistryIds = []*string{aws.String(c.registryID)}
	}

	result, err := c.client.GetAuthorizationToken(input)
	if err != nil {
		logrus.Errorf("failed to get ECR authorization token: %v", err)
		return "", ""
	}

	if len(result.AuthorizationData) == 0 {
		logrus.Error("no authorization data returned from ECR")
		return "", ""
	}

	authData := result.AuthorizationData[0]
	token := aws.StringValue(authData.AuthorizationToken)
	expiresAt := aws.TimeValue(authData.ExpiresAt)

	// Decode the base64 token to get username:password
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		logrus.Errorf("failed to decode ECR authorization token: %v", err)
		return "", ""
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		logrus.Error("invalid ECR authorization token format")
		return "", ""
	}

	c.username = parts[0]
	c.password = parts[1]

	// Set expiry time
	if c.lifetime != nil && *c.lifetime > 0 {
		c.expiry = now.Add(*c.lifetime)
	} else {
		// Default: refresh 1 hour before actual expiry
		c.expiry = expiresAt.Add(-time.Hour)
	}

	logrus.Debugf("ECR credentials refreshed, expires at: %v", c.expiry)
	return c.username, c.password
}

// RefreshToken implements the auth.CredentialStore interface
func (c *ecrCredentials) RefreshToken(_ *url.URL, _ string) string {
	return ""
}

// SetRefreshToken implements the auth.CredentialStore interface
func (c *ecrCredentials) SetRefreshToken(_ *url.URL, _, _ string) {
}

// parseECRURL extracts account ID and region from an ECR registry URL
func parseECRURL(registryURL string) (accountID, region string, err error) {
	u, err := url.Parse(registryURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid registry URL: %v", err)
	}

	matches := ecrURLPattern.FindStringSubmatch(u.Host)
	if len(matches) != 3 {
		return "", "", fmt.Errorf("URL does not match ECR registry pattern: %s", u.Host)
	}

	return matches[1], matches[2], nil
}

// configureECRAuth creates ECR credentials for the given configuration
func configureECRAuth(cfg configuration.ECRConfig, remoteURL string) (auth.CredentialStore, error) {
	// Parse account ID and region from remote URL if not provided
	accountID := cfg.AccountID
	region := cfg.Region

	if accountID == "" || region == "" {
		parsedAccountID, parsedRegion, err := parseECRURL(remoteURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ECR URL %s: %v", remoteURL, err)
		}
		if accountID == "" {
			accountID = parsedAccountID
		}
		if region == "" {
			region = parsedRegion
		}
	}

	// Create AWS session with the specified configuration
	config := &aws.Config{
		Region: aws.String(region),
	}

	// Set up credentials if provided
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		config.Credentials = awsCredentials.NewStaticCredentials(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			cfg.SessionToken,
		)
	} else if cfg.Profile != "" {
		config.Credentials = awsCredentials.NewSharedCredentials("", cfg.Profile)
	}
	// If no explicit credentials, will use AWS credential chain

	sess, err := session.NewSession(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}

	ecrClient := ecr.New(sess)

	return &ecrCredentials{
		client:     ecrClient,
		registryID: accountID,
		lifetime:   cfg.Lifetime,
	}, nil
}

// isECRURL determines if a URL is an AWS ECR registry URL
func isECRURL(registryURL string) bool {
	u, err := url.Parse(registryURL)
	if err != nil {
		return false
	}
	return ecrURLPattern.MatchString(u.Host)
}
