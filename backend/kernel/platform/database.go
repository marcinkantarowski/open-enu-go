package platform

import (
	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/search"
)

// WithDatabase attaches the database half of the platform to s: the pool,
// the command bus (writing the trail through s.Audit, naming the actor
// through s.Actor), the outbox (pushing broadcasts through s.Events) and
// the search indexer. River's tables must exist (migrate.Run).
//
// With persistAudit the audit logger is replaced by audit.Postgres, whatever
// it was before; a test that wants to assert on the trail passes false and
// keeps its Recorder. The bus is built from s.Audit after that choice, so it
// records wherever the final logger points.
func WithDatabase(s *Services, pool *scope.Pool, opts outbox.Options, persistAudit bool) error {
	s.DB = pool
	if persistAudit {
		s.Audit = audit.Postgres{Pool: pool, Log: s.Log}
	}
	s.Bus = command.NewBus(pool, s.Audit, s.Actor)
	ob, err := outbox.New(pool, s.Events, s.Log, opts)
	if err != nil {
		return err
	}
	s.Outbox = ob
	s.Search = search.Indexer{}
	return nil
}
