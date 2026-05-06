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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	authType := auth.Type
	if authType == "" {
		authType = "auto"
	}

	creds, err := resolveCredentials(ctx, authType, auth)
	if err != nil {
		return nil, err
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

// resolveCredentials selects yc-sdk credentials based on auth.Type.
//
// Token clarification:
//   - YC_OAUTH_TOKEN / auth.type=oauth: a long-lived (~1y) Yandex Passport
//     OAuth token. Issued by Yandex's OAuth server (not Yandex Cloud).
//     The SDK uses it to mint short-lived IAM tokens on each request.
//   - YC_IAM_TOKEN  / auth.type=iam_token: a short-lived (~12h) IAM token
//     specific to Yandex Cloud, produced by `yc iam create-token`.
//
// auth.type=auto resolves them transparently and additionally falls back
// to invoking the local `yc` CLI, which is exactly the path the user
// already used to authenticate to their cloud (`yc init` browser flow).
func resolveCredentials(ctx context.Context, authType string, auth AuthConfig) (ycsdk.Credentials, error) {
	switch authType {
	case "auto":
		return autoCredentials(ctx, auth)
	case "oauth":
		token := firstNonEmpty(auth.Token, os.Getenv("YC_OAUTH_TOKEN"))
		if token == "" {
			return nil, errors.New(
				"OAuth token not provided: set auth.token in config or YC_OAUTH_TOKEN env var; " +
					"YC_OAUTH_TOKEN is a long-lived Yandex Passport token, not an IAM token; " +
					"get one at https://oauth.yandex.ru/authorize?response_type=token&client_id=1a6990aa636648e9b2ef855fa7bec2fb; " +
					"or use auth.type: auto to let jtsekret invoke `yc iam create-token` for you")
		}
		return ycsdk.OAuthToken(token), nil
	case "iam_token":
		token := firstNonEmpty(auth.Token, os.Getenv("YC_IAM_TOKEN"))
		if token == "" {
			return nil, errors.New(
				"IAM token not provided: set auth.token in config or YC_IAM_TOKEN env var; " +
					"get one with `yc iam create-token` (12h lifetime); " +
					"or use auth.type: auto to make jtsekret refresh it on every call")
		}
		return ycsdk.NewIAMTokenCredentials(token), nil
	case "service_account_key":
		keyFile := firstNonEmpty(auth.ServiceAccountFile, os.Getenv("YC_SERVICE_ACCOUNT_KEY_FILE"))
		if keyFile == "" {
			return nil, errors.New("service account key file not provided: set auth.service_account_file or YC_SERVICE_ACCOUNT_KEY_FILE")
		}
		iamKey, err := iamkey.ReadFromJSONFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		c, err := ycsdk.ServiceAccountKey(iamKey)
		if err != nil {
			return nil, fmt.Errorf("service account credentials: %w", err)
		}
		return c, nil
	case "instance_service_account":
		return ycsdk.InstanceServiceAccount(), nil
	default:
		return nil, fmt.Errorf("unsupported auth.type: %q (supported: auto, oauth, iam_token, service_account_key, instance_service_account)", authType)
	}
}

// autoCredentials picks the best available credential source without
// forcing the user to declare it explicitly. Probe order:
//
//  1. explicit auth.token in config (treated as IAM if the value looks
//     like an IAM token, otherwise OAuth)
//  2. YC_IAM_TOKEN env var
//  3. YC_OAUTH_TOKEN env var
//  4. YC_SERVICE_ACCOUNT_KEY_FILE env var (or auth.service_account_file)
//  5. local `yc iam create-token` CLI invocation (uses whichever profile
//     the user already authenticated with via `yc init`)
//
// If everything fails the returned error explains how to fix it.
func autoCredentials(ctx context.Context, auth AuthConfig) (ycsdk.Credentials, error) {
	if auth.Token != "" {
		return ycsdk.NewIAMTokenCredentials(auth.Token), nil
	}
	if t := os.Getenv("YC_IAM_TOKEN"); t != "" {
		return ycsdk.NewIAMTokenCredentials(t), nil
	}
	if t := os.Getenv("YC_OAUTH_TOKEN"); t != "" {
		return ycsdk.OAuthToken(t), nil
	}
	if keyFile := firstNonEmpty(auth.ServiceAccountFile, os.Getenv("YC_SERVICE_ACCOUNT_KEY_FILE")); keyFile != "" {
		iamKey, err := iamkey.ReadFromJSONFile(keyFile)
		if err == nil {
			if c, err := ycsdk.ServiceAccountKey(iamKey); err == nil {
				return c, nil
			}
		}
	}
	token, err := iamTokenFromYCCLI(ctx)
	if err == nil {
		return ycsdk.NewIAMTokenCredentials(token), nil
	}
	if !errors.Is(err, errYCNotFound) {
		return nil, fmt.Errorf(
			"auth.type=auto: failed to obtain a token via `yc iam create-token`: %w; "+
				"run `yc init` to (re-)authenticate with Yandex Cloud, "+
				"or set YC_IAM_TOKEN / YC_OAUTH_TOKEN / a service account key explicitly", err)
	}
	return nil, errors.New(
		"auth.type=auto: no Yandex Cloud credentials found; one of the following is required: " +
			"install the `yc` CLI and run `yc init` (then jtsekret picks up auth automatically); " +
			"or set YC_IAM_TOKEN (12h lifetime, from `yc iam create-token`); " +
			"or set YC_OAUTH_TOKEN (long-lived, from https://oauth.yandex.ru/authorize?response_type=token&client_id=1a6990aa636648e9b2ef855fa7bec2fb); " +
			"or set YC_SERVICE_ACCOUNT_KEY_FILE pointing to a JSON service-account key")
}

var errYCNotFound = errors.New("yc CLI not found in PATH")

// iamTokenFromYCCLI shells out to `yc iam create-token`. Returns
// errYCNotFound if the CLI is not on PATH, allowing autoCredentials to
// fall through to its terminal error message.
func iamTokenFromYCCLI(ctx context.Context) (string, error) {
	if _, err := exec.LookPath("yc"); err != nil {
		return "", errYCNotFound
	}
	out, err := exec.CommandContext(ctx, "yc", "iam", "create-token").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("yc iam create-token: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("yc iam create-token: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

