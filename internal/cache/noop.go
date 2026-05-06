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
package cache

import (
	"context"

	"github.com/jtprogru/jtsekret/internal/domain"
)

type Noop struct{}

func NewNoop() *Noop {
	return &Noop{}
}

func (n *Noop) Get(ctx context.Context, nameOrID string) (*domain.CachedPayload, error) {
	return nil, nil //nolint:nilnil // no-op cache always reports a miss; (nil, nil) is the contract
}

func (n *Noop) Set(ctx context.Context, nameOrID string, payload *domain.CachedPayload) error {
	return nil
}

func (n *Noop) Delete(ctx context.Context, nameOrID string) error {
	return nil
}

func (n *Noop) Clear(ctx context.Context) error {
	return nil
}

func (n *Noop) Stats(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{
		"enabled": false,
	}, nil
}
