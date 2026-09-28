package requiem

import (
	"fmt"
	"time"

	"github.com/nicklecoder/requiem/internal/reach"
)

// requiem: embedding/unreachable-endpoints-remembered
// endpointDown reports whether endpoint failed to connect recently enough
// for a command that can do without it to skip it. A broken index reads as
// "not down": the worst case is one more 2-second try.
func (s *Service) endpointDown(endpoint string) (time.Time, bool) {
	ix, err := s.openIndex()
	if err != nil {
		return time.Time{}, false
	}
	defer ix.Close()
	until, down, err := ix.EndpointDownUntil(endpoint)
	if err != nil {
		return time.Time{}, false
	}
	return until, down
}

// recordReach remembers what a call to endpoint showed: an endpoint that
// could not be reached is marked down, one that answered is cleared. Any
// other failure (a slow answer, an error the server returned) says nothing
// about reachability and leaves the record alone.
func (s *Service) recordReach(endpoint string, err error) {
	ix, oerr := s.openIndex()
	if oerr != nil {
		return
	}
	defer ix.Close()
	switch {
	case err == nil:
		ix.ClearEndpoint(endpoint)
	case reach.Unreachable(err):
		ix.MarkEndpointDown(endpoint, time.Now().Add(reach.DownFor))
	}
}

// downNote is the one line a degraded command prints for a skipped endpoint.
func downNote(what, endpoint string, until time.Time) string {
	return fmt.Sprintf("%s %s could not be reached recently; skipping it until %s", what, endpoint, until.Local().Format("15:04"))
}
