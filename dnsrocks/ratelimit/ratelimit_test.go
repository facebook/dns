package ratelimit

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
)

func TestConcurrencyReader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, err := NewConcurrencyHandler(3)
	require.NoError(t, err)
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
			go h.ServeDNS(context.Background(), nil, nil)
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
