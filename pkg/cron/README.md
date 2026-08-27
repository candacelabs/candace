# Candace cron

`cron` is a durable in-process scheduler for Candace Go services. It runs as
part of the service lifecycle; it is not a separate daemon. A `Store` is
required, and PostgreSQL is the production implementation.

```go
import (
	"context"
	"database/sql"
	"time"

	cron "github.com/candacelabs/candace/pkg/cron"
	cronpostgres "github.com/candacelabs/candace/pkg/cron/postgres"
	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/sync/errgroup"
)

func run(ctx context.Context, databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	store, err := cronpostgres.NewStore(db)
	if err != nil {
		return err
	}
	scheduler, err := cron.New(
		cron.WithStore(store),
		cron.WithJob(
			"daily-rollup",
			cron.Spec(cron.Daily(cron.At(3).AM())),
			buildDailyRollup,
			cron.WithCatchUp(cron.CatchUpAll),
			cron.WithOverlap(cron.OverlapAllow),
		),
		cron.WithJob(
			"cache-refresh",
			cron.Spec(cron.Every(15*time.Minute)),
			refreshCache,
		),
	)
	if err != nil {
		return err
	}

	router := gin.New()
	scheduler.Register(router.Group("/internal")) // GET /internal/cron only

	group, groupContext := errgroup.WithContext(ctx)
	group.Go(func() error { return scheduler.Run(groupContext) })
	group.Go(func() error { return serveHTTP(groupContext, router) })
	return group.Wait()
}

func buildDailyRollup(ctx context.Context, invocation cron.Invocation) error {
	// invocation.ID is the stable idempotency key for this scheduled instant.
	return nil
}
```

Apply the relational migration in
`postgres/migrations/000001_create_cron_jobs_and_runs.up.sql` through the
owning service's normal migration runner. Queries are generated and checked in
with:

```sh
./pkg/cron/postgres/generate.sh write
./pkg/cron/postgres/generate.sh check
```

The PostgreSQL integration suite is opt-in locally. Point it at a disposable
database whose name ends in `_test`, then run the complete cron suite from the
`candace` module root:

```sh
CANDACE_CRON_TEST_DATABASE_URL='postgresql://cron:cron@localhost:5432/candace_cron_test?sslmode=disable' \
  go test -race ./pkg/cron/...
```

The integration harness rejects database names without the `_test` suffix and
runs each suite in a unique schema that it removes afterward. It never reuses
the scheduler tables in `public`.

The PostgreSQL model is ordinary typed relational state: job definitions,
schedule columns, cursors, occurrences, attempts, and fenced leases. The
Liquid Proto messages under `cron/v1` are only portable HTTP or
messaging contracts; protobuf wire bytes are never stored in the database.

Execution is at least once. A lease can expire after the handler produced an
external side effect but before completion was recorded, so handlers must use
`Invocation.ID` as an idempotency key. `CatchUpNone`, `CatchUpLatest`, and
`CatchUpAll` control missed occurrences; `OverlapSkip` is conservative by
default, while `OverlapAllow` permits concurrent attempts. Handlers must honor
context cancellation; `Run` drains cooperative handlers before returning.

The status snapshot includes active jobs and the 1,000 most recent durable
occurrences, keeping the read-only Gin endpoint bounded as history grows.

For tests or deliberately disposable processes, opt into memory explicitly:

```go
scheduler, err := cron.New(
	cron.WithStore(cron.NewMemoryStore()),
	cron.WithJob("test-job", cron.Spec(cron.Daily(cron.Noon())), handler),
)
```

`MemoryStore` never pretends to be durable and is never selected implicitly.
