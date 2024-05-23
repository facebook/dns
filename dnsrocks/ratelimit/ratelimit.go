/*
Copyright (c) Meta Platforms, Inc. and affiliates.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ratelimit

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
	"golang.org/x/sync/semaphore"
)

// ConcurrencyHandler is a [plugin.Handler] that limits how many queries
// are currently being processed.  It should be the first handler in the
// chain, and must be used with the associated [ConcurrencyLimitingReader].
//
// ConcurrencyHandler does not provide an absolute guarantee, because each
// query is marked as "completed" just _before_ its goroutine terminates.
type ConcurrencyHandler struct {
	// A Go semaphore is just a locked FIFO queue.
	// TODO: Upgrade this to a fair queue.
	sem  *semaphore.Weighted
	Next plugin.Handler
}

// NewConcurrencyHandler initializes a new concurrency-limiting Handler.
func NewConcurrencyHandler(maxWorkers int) (*ConcurrencyHandler, error) {
	if maxWorkers < 1 {
		return nil, fmt.Errorf("invalid number of workers: %d", maxWorkers)
	}
	return &ConcurrencyHandler{
		sem: semaphore.NewWeighted(int64(maxWorkers)),
	}, nil
}

// ServeDNS implements the [plugin.Handler] interface.
func (h *ConcurrencyHandler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	code, err := plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	h.sem.Release(1)
	return code, err
}

// Name returns the handler's name.
func (h *ConcurrencyHandler) Name() string { return "concurrency" }

// Acquire must be called before each call to ServeDNS.
func (h *ConcurrencyHandler) Acquire(ctx context.Context) error {
	return h.sem.Acquire(ctx, 1)
}

// DecorateReader returns a ConcurrencyLimitingReader connected to this ConcurrencyHandler.
func (h *ConcurrencyHandler) DecorateReader(inner dns.Reader) dns.Reader {
	return newDelayLimitingReader(inner.(dns.PacketConnReader), h)
}

// ConcurrencyLimitingReader is a [dns.Reader] that drops packets when
// the concurrency exceeds some threshold.
type ConcurrencyLimitingReader struct {
	inner   dns.PacketConnReader
	handler *ConcurrencyHandler
}

func newDelayLimitingReader(inner dns.PacketConnReader, handler *ConcurrencyHandler) *ConcurrencyLimitingReader {
	return &ConcurrencyLimitingReader{
		inner:   inner,
		handler: handler,
	}
}

func (r *ConcurrencyLimitingReader) acquire(deadline time.Time) error {
	ctx := context.Background()
	if !deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	return r.handler.Acquire(ctx)
}

func deadline(timeout time.Duration) time.Time {
	if timeout > 0 {
		return time.Now().Add(timeout)
	}
	return time.Time{}
}

// ReadUDP implements [dns.Reader].
func (r *ConcurrencyLimitingReader) ReadUDP(conn *net.UDPConn, timeout time.Duration) ([]byte, *dns.SessionUDP, error) {
	deadline := deadline(timeout)
	queryBuf, s, err := r.inner.ReadUDP(conn, timeout)
	if err != nil {
		return nil, nil, err
	}

	if err = r.acquire(deadline); err != nil {
		return nil, nil, err
	}

	return queryBuf, s, nil
}

// ReadPacketConn implements [dns.PacketConnReader].
func (r *ConcurrencyLimitingReader) ReadPacketConn(conn net.PacketConn, timeout time.Duration) ([]byte, net.Addr, error) {
	deadline := deadline(timeout)
	queryBuf, addr, err := r.inner.ReadPacketConn(conn, timeout)
	if err != nil {
		return nil, addr, err
	}

	if err = r.acquire(deadline); err != nil {
		return nil, nil, err
	}

	return queryBuf, addr, nil
}

// ReadTCP implements [dns.Reader].
func (r *ConcurrencyLimitingReader) ReadTCP(conn net.Conn, timeout time.Duration) ([]byte, error) {
	deadline := deadline(timeout)
	queryBuf, err := r.inner.ReadTCP(conn, timeout)
	if err != nil {
		return nil, err
	}

	if err = r.acquire(deadline); err != nil {
		return nil, err
	}

	return queryBuf, nil
}
