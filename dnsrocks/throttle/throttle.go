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
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/facebook/dns/dnsrocks/dnsserver/stats"
	"github.com/miekg/dns"
	"golang.org/x/sync/semaphore"
)

const statName = "DNS.active_queries_pct"

// Limiter is a wrapped Semaphore that exposes a counter.
type Limiter struct {
	// A Go semaphore is just a locked FIFO queue.
	// TODO: Upgrade this to a fair queue.
	sem   *semaphore.Weighted
	limit int64
	// Semaphores don't expose the current acquired value, so we have to
	// track it separately.  This tracking isn't synchronized with the
	// semaphore, but that's fine because it's only for monitoring.
	count atomic.Int64
}

// Acquire must be called before accepting each incoming query packet.
func (l *Limiter) acquire(ctx context.Context) error {
	err := l.sem.Acquire(ctx, 1)
	if err == nil {
		l.count.Add(1)
	}
	return err
}

func (l *Limiter) release() {
	l.sem.Release(1)
	l.count.Add(-1)
}

// Count returns the number of currently active queries.
// Only for monitoring.
func (l *Limiter) Count() int64 {
	return l.count.Load()
}

// NewLimiter initializes a new concurrency-limiting Limiter.
func NewLimiter(maxWorkers int) (*Limiter, error) {
	if maxWorkers < 1 {
		return nil, fmt.Errorf("invalid number of workers: %d", maxWorkers)
	}
	limit := int64(maxWorkers)
	return &Limiter{
		sem:   semaphore.NewWeighted(limit),
		limit: limit,
	}, nil
}

// Handler is a [plugin.Handler] that limits how many queries
// are currently being processed.  It should be the first handler in the
// chain, and must be used with the associated [Reader].
//
// Handler does not provide an absolute guarantee, because each
// query is marked as "completed" just _before_ its goroutine terminates.
type Handler struct {
	lim  *Limiter
	Next plugin.Handler
}

// NewHandler initializes a new concurrency-limiting Handler.
func NewHandler(lim *Limiter) *Handler {
	return &Handler{lim: lim}
}

// ServeDNS implements the [plugin.Handler] interface.
func (h *Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	defer h.lim.release()
	return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
}

// Name returns the handler's name.
func (h *Handler) Name() string { return "throttle" }

// DecorateReader is a [dns.DecorateReader].
func (h *Handler) DecorateReader(inner dns.Reader) dns.Reader {
	return newReader(inner.(dns.PacketConnReader), h)
}

// MsgInvalid is a [dns.MsgInvalidFunc].
func (h *Handler) MsgInvalid([]byte, error) {
	// If an invalid packet arrives, throttle.Reader will acquire
	// the semaphore, but ServeDNS will never be called to release it.
	// Instead, we need to release it here.
	h.lim.release()
}

// MsgAcceptFunc is a [dns.MsgAcceptFunc].
func (h *Handler) MsgAcceptFunc(dh dns.Header) dns.MsgAcceptAction {
	action := dns.DefaultMsgAcceptFunc(dh)
	if action != dns.MsgAccept {
		h.lim.release()
	}
	return action
}

// Attach connects this handler to a Server's message processing path.
// This method must be called before the server starts.
func (h *Handler) Attach(s *dns.Server) {
	s.DecorateReader = h.DecorateReader
	s.MsgInvalidFunc = h.MsgInvalid
	s.MsgAcceptFunc = h.MsgAcceptFunc
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
	return r.handler.lim.acquire(ctx)
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

// Monitor the number of active queries and record it in the stats
// as a percentage of the maximum allowed.
func Monitor(lim *Limiter, stats stats.Stats, interval time.Duration) {
	for range time.Tick(interval) {
		stats.AddSample(statName, (100*lim.Count())/lim.limit)
	}
}
