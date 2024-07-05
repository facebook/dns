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

package throttle

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/miekg/dns"
	"golang.org/x/sync/semaphore"
)

// Handler is a [plugin.Handler] that limits how many queries
// are currently being processed.  It should be the first handler in the
// chain, and must be used with the associated [Reader].
//
// Handler does not provide an absolute guarantee, because each
// query is marked as "completed" just _before_ its goroutine terminates.
type Handler struct {
	// A Go semaphore is just a locked FIFO queue.
	// TODO: Upgrade this to a fair queue.
	sem  *semaphore.Weighted
	Next plugin.Handler
}

// NewHandler initializes a new concurrency-limiting Handler.
func NewHandler(maxWorkers int) (*Handler, error) {
	if maxWorkers < 1 {
		return nil, fmt.Errorf("invalid number of workers: %d", maxWorkers)
	}
	return &Handler{
		sem: semaphore.NewWeighted(int64(maxWorkers)),
	}, nil
}

// ServeDNS implements the [plugin.Handler] interface.
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	defer h.sem.Release(1)
	return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
}

// Name returns the handler's name.
func (h *Handler) Name() string { return "throttle" }

// Acquire must be called before accepting each incoming query packet.
func (h *Handler) Acquire(ctx context.Context) error {
	return h.sem.Acquire(ctx, 1)
}

// DecorateReader is a [dns.DecorateReader].
func (h *Handler) DecorateReader(inner dns.Reader) dns.Reader {
	return newReader(inner.(dns.PacketConnReader), h)
}

// MsgInvalid is a [dns.MsgInvalidFunc].
func (h *Handler) MsgInvalid([]byte, error) {
	// If an invalid packet arrives, throttle.Reader will acquire
	// the semaphore, but ServeDNS will never be called to release it.
	// Instead, we need to release it here.
	h.sem.Release(1)
}

// Attach connects this handler to a Server's message processing path.
// This method must be called before the server starts.
func (h *Handler) Attach(s *dns.Server) {
	s.DecorateReader = h.DecorateReader
	s.MsgInvalidFunc = h.MsgInvalid
}

// Reader is a [dns.Reader] that blocks when
// the concurrency exceeds some threshold.
type Reader struct {
	inner   dns.PacketConnReader
	handler *Handler
}

func newReader(inner dns.PacketConnReader, handler *Handler) *Reader {
	return &Reader{
		inner:   inner,
		handler: handler,
	}
}

func (r *Reader) acquire(deadline time.Time) error {
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
func (r *Reader) ReadUDP(conn *net.UDPConn, timeout time.Duration) ([]byte, *dns.SessionUDP, error) {
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
func (r *Reader) ReadPacketConn(conn net.PacketConn, timeout time.Duration) ([]byte, net.Addr, error) {
	deadline := deadline(timeout)
	queryBuf, addr, err := r.inner.ReadPacketConn(conn, timeout)
	if err != nil {
		return nil, nil, err
	}

	if err = r.acquire(deadline); err != nil {
		return nil, nil, err
	}

	return queryBuf, addr, nil
}

// ReadTCP implements [dns.Reader].
func (r *Reader) ReadTCP(conn net.Conn, timeout time.Duration) ([]byte, error) {
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
