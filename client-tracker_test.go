package torrent

import (
	"github.com/anacrolix/torrent/internal/testutil"
	"github.com/anacrolix/torrent/webtorrent"
	"github.com/stretchr/testify/require"
	"net"
	"os"
	"testing"
)

func TestClientInvalidTracker(t *testing.T) {
	cfg := TestingConfig(t)
	cfg.DisableTrackers = false
	cfg.Observers = &Observers{
		Trackers: webtorrent.TrackerObserver{
			ConnStatus: make(chan webtorrent.TrackerStatus),
		},
	}

	cl, err := NewClient(cfg)
	require.NoError(t, err)
	defer cl.Close()

	dir, mi := testutil.GreetingTestTorrent()
	defer os.RemoveAll(dir)

	mi.AnnounceList = [][]string{
		{"ws://test.invalid:4242"},
	}

	to, err := cl.AddTorrent(mi)
	require.NoError(t, err)

	status := <-cfg.Observers.Trackers.ConnStatus
	require.Equal(t, "ws://test.invalid:4242", status.Url)
	var expected *net.OpError
	require.ErrorAs(t, expected, &status.Err)

	to.Drop()
}
