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
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/semaphore"
)

func TestConcurrencyReader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	l, err := NewLimiter(3)
	require.NoError(t, err)
	h := NewHandler(l)
	next := NewMockHandler(ctrl)
	h.Next = next
	innerReader := NewMockPacketConnReader(ctrl)
	reader := h.DecorateReader(innerReader)
	var conn net.PacketConn

	var buf [32]byte
	addr := &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}

	doWrite := make(chan struct{})
	sem := semaphore.NewWeighted(3)

	innerReader.EXPECT().ReadPacketConn(conn, time.Duration(0)).Times(10).Return(buf[:], addr, nil)
	next.EXPECT().Name().AnyTimes().Return("mock")
	next.EXPECT().ServeDNS(gomock.Any(), gomock.Any(), gomock.Any()).Times(10).Do(func(_, _, _ interface{}) {
		require.True(t, sem.TryAcquire(1), "Too many concurrent requests")
		<-doWrite
		sem.Release(1)
	})

	go func() {
		for i := 0; i < 10; i++ {
			outBuf, outAddr, err := reader.(dns.PacketConnReader).ReadPacketConn(conn, 0)
			require.NoError(t, err)
			require.Equal(t, &buf[0], &outBuf[0])
			require.Equal(t, outAddr, addr)
			go func() {
				_, err := h.ServeDNS(context.Background(), nil, nil)
				require.NoError(t, err)
			}()
		}
	}()

	// Wait for three concurrent requests to be issued.
	for sem.TryAcquire(1) {
		sem.Release(1)
		time.Sleep(time.Millisecond)
	}

	for i := 0; i < 10; i++ {
		doWrite <- struct{}{}
	}
}

// Verify that context cancellation causes Acquire() to return early
// with an error.
func TestContext(t *testing.T) {
	l, err := NewLimiter(3)
	require.NoError(t, err)

	for range 3 {
		err = l.acquire(context.Background())
		require.NoError(t, err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	err = l.acquire(canceled)
	require.Error(t, err)
}

func TestMsgInvalid(t *testing.T) {
	l, err := NewLimiter(3)
	require.NoError(t, err)
	h := NewHandler(l)

	// This handler is limited to three concurrent requests,
	// but calling MsgInvalid decrements the counter so we can
	// do this loop as many times as we want.
	for range 10 {
		err = l.acquire(context.Background())
		require.NoError(t, err)
		h.MsgInvalid(nil, nil)
	}
}

func TestCount(t *testing.T) {
	l, err := NewLimiter(3)
	require.NoError(t, err)
	require.EqualValues(t, 0, l.Count())

	ctx, cancel := context.WithCancel(context.Background())

	// Add three queries.
	err = l.acquire(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, l.Count())

	err = l.acquire(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, l.Count())

	err = l.acquire(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 3, l.Count())

	// Remove three queries.
	l.release()
	require.EqualValues(t, 2, l.Count())

	l.release()
	require.EqualValues(t, 1, l.Count())

	l.release()
	require.EqualValues(t, 0, l.Count())

	// Add one query again.
	err = l.acquire(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, l.Count())

	// Cancel the context.
	cancel()

	// Count should not be incremented if the context was canceled.
	err = l.acquire(ctx)
	require.Error(t, err)
	require.EqualValues(t, 1, l.Count())
}
