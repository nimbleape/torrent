package torrent

import (
	"io"
	"os"
	"testing"
	"testing/iotest"
	"time"

	"github.com/anacrolix/missinggo/v2/bitmap"
	"github.com/anacrolix/torrent/internal/testutil"
	"github.com/frankban/quicktest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
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
	require.Equal(t, "", status.Err)
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
			Err: "test error",
		}
	}()

	status := readChannelTimeout(t, cfg.Observers.Peers.PeerStatus, 500*time.Millisecond).(PeerStatus)
	require.False(t, status.Ok)
	require.Equal(t, status.Err, "test error")
}

func TestPeerConnEstablished(t *testing.T) {
	obs := NewClientObservers()
	ps := testClientTransferParams{
		ConfigureSeeder: ConfigureClient{
			Config: func(cfg *ClientConfig) {
				cfg.PeerID = "12345123451234512345"
			},
		},
		ConfigureLeecher: ConfigureClient{
			Config: func(cfg *ClientConfig) {
				// TODO one of UTP or TCP is needed for the transfer
				// Does this mean we're not doing webtorrent? TBC
				// cfg.DisableUTP = true
				cfg.DisableTCP = true
				cfg.Debug = false
				cfg.DisableTrackers = true
				cfg.EstablishedConnsPerTorrent = 1
				cfg.Observers = obs
			},
		},
	}

	go testClientTransfer(t, ps)

	status := readChannelTimeout(t, obs.Peers.PeerStatus, 500*time.Millisecond).(PeerStatus)
	// FIXME converting [20]byte to string is not enough to pass the test
	// require.Equal(t, "12345123451234512345", fmt.Sprintf("%+q", status.Id))
	require.True(t, status.Ok)
	require.Equal(t, "", status.Err)

	// Peer conn is dropped after transfer is finished. This is the next update we receive.
	status = readChannelTimeout(t, obs.Peers.PeerStatus, 500*time.Millisecond).(PeerStatus)
	// TODO a check on PeerID
	require.False(t, status.Ok)
	require.Equal(t, "", status.Err)
}

type ConfigureClient struct {
	Config func(cfg *ClientConfig)
	Client func(cl *Client)
}

type testClientTransferParams struct {
	SeederUploadRateLimiter    *rate.Limiter
	LeecherDownloadRateLimiter *rate.Limiter
	ConfigureSeeder            ConfigureClient
	ConfigureLeecher           ConfigureClient

	LeecherStartsWithoutMetadata bool
}

// Simplified version of testClientTransfer found in test/leecher-storage.go.
// Could not import and reuse that function due to circular dependencies between modules.
func testClientTransfer(t *testing.T, ps testClientTransferParams) {
	greetingTempDir, mi := testutil.GreetingTestTorrent()
	defer os.RemoveAll(greetingTempDir)
	// Create seeder and a Torrent.
	cfg := TestingConfig(t)
	cfg.Seed = true
	// Some test instances don't like this being on, even when there's no cache involved.
	cfg.DropMutuallyCompletePeers = false
	if ps.SeederUploadRateLimiter != nil {
		cfg.UploadRateLimiter = ps.SeederUploadRateLimiter
	}
	cfg.DataDir = greetingTempDir
	if ps.ConfigureSeeder.Config != nil {
		ps.ConfigureSeeder.Config(cfg)
	}
	seeder, err := NewClient(cfg)
	require.NoError(t, err)
	if ps.ConfigureSeeder.Client != nil {
		ps.ConfigureSeeder.Client(seeder)
	}
	seederTorrent, _, _ := seeder.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	defer seeder.Close()
	<-seederTorrent.Complete.On()

	// Create leecher and a Torrent.
	leecherDataDir := t.TempDir()
	cfg = TestingConfig(t)
	// See the seeder client config comment.
	cfg.DropMutuallyCompletePeers = false
	cfg.DataDir = leecherDataDir
	if ps.LeecherDownloadRateLimiter != nil {
		cfg.DownloadRateLimiter = ps.LeecherDownloadRateLimiter
	}
	cfg.Seed = false
	if ps.ConfigureLeecher.Config != nil {
		ps.ConfigureLeecher.Config(cfg)
	}
	leecher, err := NewClient(cfg)
	require.NoError(t, err)
	defer leecher.Close()
	if ps.ConfigureLeecher.Client != nil {
		ps.ConfigureLeecher.Client(leecher)
	}
	leecherTorrent, new, err := leecher.AddTorrentSpec(func() (ret *TorrentSpec) {
		ret = TorrentSpecFromMetaInfo(mi)
		ret.ChunkSize = 2
		if ps.LeecherStartsWithoutMetadata {
			ret.InfoBytes = nil
		}
		return
	}())
	require.NoError(t, err)
	assert.False(t, leecherTorrent.Complete.Bool())
	assert.True(t, new)

	added := leecherTorrent.AddClientPeer(seeder)
	assert.False(t, leecherTorrent.Seeding())
	// The leecher will use peers immediately if it doesn't have the metadata. Otherwise, they
	// should be sitting idle until we demand data.
	if !ps.LeecherStartsWithoutMetadata {
		assert.EqualValues(t, added, leecherTorrent.Stats().PendingPeers)
	}
	if ps.LeecherStartsWithoutMetadata {
		<-leecherTorrent.GotInfo()
	}
	r := leecherTorrent.NewReader()
	defer r.Close()
	go leecherTorrent.SetInfoBytes(mi.InfoBytes)

	assertReadAllGreeting(t, r)
	<-leecherTorrent.Complete.On()
	assert.NotEmpty(t, seederTorrent.PeerConns())
	leecherPeerConns := leecherTorrent.PeerConns()
	if cfg.DropMutuallyCompletePeers {
		// I don't think we can assume it will be empty already, due to timing.
		// assert.Empty(t, leecherPeerConns)
	} else {
		assert.NotEmpty(t, leecherPeerConns)
	}
	foundSeeder := false
	for _, pc := range leecherPeerConns {
		completed := pc.PeerPieces().GetCardinality()
		t.Logf("peer conn %v has %v completed pieces", pc, completed)
		if completed == bitmap.BitRange(leecherTorrent.Info().NumPieces()) {
			foundSeeder = true
		}
	}
	if !foundSeeder {
		t.Errorf("didn't find seeder amongst leecher peer conns")
	}

	seederStats := seederTorrent.Stats()
	assert.True(t, 13 <= seederStats.BytesWrittenData.Int64())
	assert.True(t, 8 <= seederStats.ChunksWritten.Int64())

	leecherStats := leecherTorrent.Stats()
	assert.True(t, 13 <= leecherStats.BytesReadData.Int64())
	assert.True(t, 8 <= leecherStats.ChunksRead.Int64())

	// Try reading through again for the cases where the torrent data size
	// exceeds the size of the cache.
	assertReadAllGreeting(t, r)
}

func assertReadAllGreeting(t *testing.T, r io.ReadSeeker) {
	pos, err := r.Seek(0, io.SeekStart)
	assert.NoError(t, err)
	assert.EqualValues(t, 0, pos)
	quicktest.Check(t, iotest.TestReader(r, []byte(testutil.GreetingFileContents)), quicktest.IsNil)
}