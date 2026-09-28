package requiem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nicklecoder/requiem/internal/classify"
	"github.com/nicklecoder/requiem/internal/config"
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

// errNoClassifierAnswered means every configured classifier was skipped or
// out of reach; its message says why each one was passed over.
type errNoClassifierAnswered struct{ why []string }

func (e errNoClassifierAnswered) Error() string { return strings.Join(e.why, "; ") }

// requiem: embedding/fallback-endpoints
// withClassifier runs fn against the configured classifier, then each
// fallback, until one answers, and returns the identity of the one that did.
// skipDown passes over classifiers recently marked unreachable, for the
// commands that can do without one; the others always try. A failure that is
// not an outage (a server error, a wrong model) is returned as it is.
func (s *Service) withClassifier(cfg *config.Config, skipDown bool, fn func(*classify.Client, config.Classifier) error) (string, error) {
	var why []string
	for _, k := range cfg.Classifiers() {
		client, err := classify.New(k)
		if err != nil {
			why = append(why, err.Error())
			continue
		}
		if skipDown {
			if until, down := s.endpointDown(client.Endpoint()); down {
				why = append(why, downNote("classifier", client.Endpoint(), until))
				continue
			}
		}
		err = fn(client, k)
		s.recordReach(client.Endpoint(), err)
		if err == nil {
			return client.Identity(), nil
		}
		if reach.Unreachable(err) || errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			why = append(why, fmt.Sprintf("classifier %s did not answer (%v)", client.Identity(), err))
			continue
		}
		return "", err
	}
	return "", errNoClassifierAnswered{why}
}
