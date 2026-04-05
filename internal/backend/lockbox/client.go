/*
Copyright © 2026 Mikhail Savin <jtprogru@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package lockbox

import (
	"context"
	"fmt"
	"os"

	"github.com/yandex-cloud/go-sdk"
	"github.com/yandex-cloud/go-sdk/iamkey"
)

type Client struct {
	sdk      *ycsdk.SDK
	folderID string
}

type AuthConfig struct {
	Type               string
	Token              string
	ServiceAccountFile string
}

func NewClient(ctx context.Context, folderID string, auth AuthConfig) (*Client, error) {
	var creds ycsdk.Credentials

	switch auth.Type {
	case "oauth":
		token := auth.Token
		if token == "" {
			token = os.Getenv("YC_OAUTH_TOKEN")
			if token == "" {
				return nil, fmt.Errorf("OAuth token not provided: set auth.token in config or YC_OAUTH_TOKEN env var")
			}
		}
		creds = ycsdk.OAuthToken(token)

	case "iam_token":
		token := auth.Token
		if token == "" {
			token = os.Getenv("YC_IAM_TOKEN")
			if token == "" {
				return nil, fmt.Errorf("IAM token not provided: set auth.token in config or YC_IAM_TOKEN env var (run: yc iam create-token)")
			}
		}
		creds = ycsdk.NewIAMTokenCredentials(token)

	case "service_account_key":
		keyFile := auth.ServiceAccountFile
		if keyFile == "" {
			keyFile = os.Getenv("YC_SERVICE_ACCOUNT_KEY_FILE")
			if keyFile == "" {
				return nil, fmt.Errorf("service account key file not provided: set auth.service_account_file in config or YC_SERVICE_ACCOUNT_KEY_FILE env var")
			}
		}
		iamKey, err := iamkey.ReadFromJSONFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		creds, err = ycsdk.ServiceAccountKey(iamKey)
		if err != nil {
			return nil, fmt.Errorf("create service account credentials: %w", err)
		}

	case "instance_service_account":
		creds = ycsdk.InstanceServiceAccount()

	default:
		return nil, fmt.Errorf("unsupported auth type: %q (supported: oauth, iam_token, service_account_key, instance_service_account)", auth.Type)
	}

	sdk, err := ycsdk.Build(ctx, ycsdk.Config{
		Credentials: creds,
	})
	if err != nil {
		return nil, fmt.Errorf("build SDK: %w", err)
	}

	return &Client{
		sdk:      sdk,
		folderID: folderID,
	}, nil
}

func (c *Client) SDK() *ycsdk.SDK {
	return c.sdk
}

func (c *Client) FolderID() string {
	return c.folderID
}

