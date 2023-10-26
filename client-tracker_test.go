package torrent

import (
	"github.com/anacrolix/torrent/internal/testutil"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestClientInvalidTracker(t *testing.T) {
	cfg := TestingConfig(t)
	cfg.DisableTrackers = false
	cfg.Observers = &Observers{
		Trackers: struct{ ConnStatus chan string }{ConnStatus: make(chan string)},
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

	require.Equal(t, "bar", <-cfg.Observers.Trackers.ConnStatus)

	to.Drop()
}
