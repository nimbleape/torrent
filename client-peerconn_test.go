package torrent

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPeerConnObserverReadStatusOk(t *testing.T) {
	cfg := TestingConfig(t)
	cfg.DisableTrackers = false
	cfg.EstablishedConnsPerTorrent = 1
	cfg.Observers = &Observers{
		Peers: PeerObserver{
			PeerStatus: make(chan PeerStatus),
		},
	}

	c, _ := NewClient(cfg)
	defer c.Close()

	go func() {
		cfg.Observers.Peers.PeerStatus <- PeerStatus{
			Ok: true,
		}
	}()

	status := readChannelTimeout(t, cfg.Observers.Peers.PeerStatus, 500*time.Millisecond).(PeerStatus)
	require.True(t, status.Ok)
	require.Nil(t, status.Err)
}

func TestPeerConnObserverReadStatusErr(t *testing.T) {
	cfg := TestingConfig(t)
	cfg.DisableTrackers = false
	cfg.EstablishedConnsPerTorrent = 1
	cfg.Observers = &Observers{
		Peers: PeerObserver{
			PeerStatus: make(chan PeerStatus),
		},
	}

	c, _ := NewClient(cfg)
	defer c.Close()

	go func() {
		cfg.Observers.Peers.PeerStatus <- PeerStatus{
			Err: errors.New("test error"),
		}
	}()

	status := readChannelTimeout(t, cfg.Observers.Peers.PeerStatus, 500*time.Millisecond).(PeerStatus)
	require.False(t, status.Ok)
	require.EqualError(t, status.Err, "test error")
}

// TODO ideally we'd run both peers locally for these tests,
// and transfer a very small file

func TestPeerConnEstablished(t *testing.T) {
	cfg := TestingConfig(t)
	cfg.DisableTrackers = false
	cfg.EstablishedConnsPerTorrent = 1
	cfg.Observers = NewClientObservers()

	c, _ := NewClient(cfg)
	defer c.Close()

	// Sintel, a free, Creative Commons movie
	const m = "magnet:?xt=urn:btih:08ada5a7a6183aae1e09d831df6748d566095a10&dn=Sintel&tr=wss%3A%2F%2Ftracker.btorrent.xyz&tr=wss%3A%2F%2Ftracker.fastcast.nz&tr=wss%3A%2F%2Ftracker.openwebtorrent.com&ws=https%3A%2F%2Fwebtorrent.io%2Ftorrents%2F&xs=https%3A%2F%2Fwebtorrent.io%2Ftorrents%2Fsintel.torrent"

	to, err := c.AddMagnet(m)
	require.NoError(t, err)

	<-to.GotInfo()
	to.DownloadAll()

	// need to give it enough time to connect to actual peers
	status := readChannelTimeout(t, cfg.Observers.Peers.PeerStatus, 60*time.Second).(PeerStatus)
	// TODO a check about PeerID?
	require.True(t, status.Ok)
	require.Nil(t, status.Err)
}
