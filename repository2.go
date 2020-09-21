package smallben

import (
	"context"
	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"time"
)

// Repository2 is used to manage the operations within the Postgres
// backend. It is created with the function `NewRepository2`.
type Repository2 struct {
	pool pgxpool.Pool
}

// NewRepository2 returns an instance of Repository2.
func NewRepository2(ctx context.Context, options *pgxpool.Config) (Repository2, error) {
	pool, err := pgxpool.ConnectConfig(ctx, options)
	if err != nil {
		return Repository2{}, err
	}
	return Repository2{
		pool: *pool,
	}, nil
}

// AddTests add `tests` within to the database. The update is done by using a batch
// in a transaction.
func (r *Repository2) AddTests(ctx context.Context, tests []Test) error {
	//	// create the batch of requests
	//	var batch pgx.Batch
	//	var result pgx.BatchResults
	//	var err error
	//
	//	defer func() {
	//		if err := result.Close(); err != nil {
	//			// log the error
	//		}
	//	}()
	//
	//	for _, test := range tests {
	//		batch.Queue(`insert into tests
	//(id, user_evaluation_rule_id, user_id, paused, every_second, created_at, updated_at)
	//values
	//($1, $2, $3, false, $4, $5, $6)
	//`, test.Id, test.UserEvaluationRuleId, test.UserId, test.EverySecond, time.Now(), time.Now())
	//	}
	//
	//	log.Print("Done building")
	//
	//	// now, send the batch requests
	//	// it is implicitly executed within a transaction.
	//	newCtx, _ := context.WithTimeout(context.Background(), 50 * time.Millisecond)
	//
	//	result = r.pool.SendBatch(newCtx, &batch)
	//
	//	log.Print("send the batch")
	//
	//	_, err = result.Exec()
	//
	//	log.Print("done exec")
	//
	//	return err
	rows := make([][]interface{}, len(tests))
	for i, test := range tests {
		rows[i] = test.addToRaw()
	}

	copyCount, err := r.pool.CopyFrom(ctx, pgx.Identifier{"tests"}, addToColumn(), pgx.CopyFromRows(rows))
	if err != nil {
		return err
	}
	if copyCount != int64(len(tests)) {
		return pgx.ErrNoRows
	}
	return nil
}

// GetTest returns a TestWithSchedule whose id is `testID`.
func (r *Repository2) GetTest(ctx context.Context, testID int32) (TestWithSchedule, error) {
	var test Test
	err := pgxscan.Select(ctx, &r.pool, &test, `select id, user_id, cron_id, every_second,
paused, created_at, updated_at, user_evaluation_rule_id from tests where id=$1`, testID)
	if err != nil {
		return TestWithSchedule{}, err
	}
	testWithSchedule, err := (&test).ToTestWithSchedule()
	return testWithSchedule, err
}

// PauseTests pauses `tests`.
func (r *Repository2) PauseTests(ctx context.Context, tests []Test) error {
	ids := GetIdsFromTestList(tests)
	_, err := r.pool.Exec(ctx, `update tests set paused = true where id = any($1)`, ids)
	if err != nil {
		return err
	}
	return err
}

// ResumeTests resumes `tests`.
func (r *Repository2) ResumeTests(ctx context.Context, tests []Test) error {
	ids := GetIdsFromTestList(tests)
	_, err := r.pool.Exec(ctx, `update tests set paused = false where id = any($1)`, ids)
	if err != nil {
		return err
	}
	return nil
}

// GetAllTestsToExecute returns all the test whose `paused` field is set to `false`.
func (r *Repository2) GetAllTestsToExecute(ctx context.Context) ([]Test, error) {
	var tests []Test
	err := pgxscan.Select(ctx, &r.pool, &tests, `select id, user_id, cron_id, every_second,
paused, created_at, updated_at, user_evaluation_rule_id from tests where paused = false`)
	return tests, err
}

// GetTestsByKeys returns all the tests whose primary key are in `testsID`. It returns an
// error of type `pgx.ErrNoRow` in case there is a mismatch between the length of the returned
// tests and of the input.
func (r *Repository2) GetTestsByKeys(ctx context.Context, testsID []int32) ([]Test, error) {
	var tests []Test
	err := pgxscan.Select(ctx, &r.pool, &tests, `select select id, user_id, cron_id, every_second,
paused, created_at, updated_at, user_evaluation_rule_id from tests where id = any($1)`, testsID)
	if err != nil {
		return nil, err
	}
	if len(tests) != len(testsID) {
		return nil, pgx.ErrNoRows
	}
	return tests, err
}

// DeleteTestsByKeys deletes the tests whose id are in `testsID`.
func (r *Repository2) DeleteTestsByKeys(ctx context.Context, testsID []int32) error {
	_, err := r.pool.Exec(ctx, "delete from tests where id = any($1)", testsID)
	if err != nil {
		return err
	}
	return nil
}

// ChangeSchedule update `tests` by saving in the database the new schedule.
// Execution is done within a transaction.
func (r *Repository2) ChangeSchedule(ctx context.Context, tests []Test) error {
	// create the batch of requests
	var batch pgx.Batch
	var result pgx.BatchResults
	var err error

	defer func() {
		if err := result.Close(); err != nil {
			// log the error
		}
	}()

	// start the transaction
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	// prepare the batch request
	for _, test := range tests {
		batch.Queue("update tests set every_second = $2, updated_at = $3, where id = $1",
			test.Id, test.EverySecond, time.Now())
	}

	// now, send the batch requests
	result = tx.SendBatch(ctx, &batch)
	_, err = result.Exec()
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			// log the rollback error, there is nothing more we can do...
		}
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		// well, what do? Just try to rollback
		if err = tx.Rollback(ctx); err != nil {
			// nothing more to do to.
		}
	}
	return err
}

func (r *Repository2) transactionUpdate(ctx context.Context, tests []Test, getBatchFn func(test *Test, batch *pgx.Batch)) error {
	// create the batch of requests
	var batch pgx.Batch
	var result pgx.BatchResults
	var err error

	defer func() {
		if err := result.Close(); err != nil {
			// log the error
		}
	}()

	// start the transaction
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	// prepare the batch request
	for _, test := range tests {
		getBatchFn(&test, &batch)
	}

	// now, send the batch requests
	result = tx.SendBatch(ctx, &batch)
	_, err = result.Exec()
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			// log the rollback error, there is nothing more we can do...
		}
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		// well, what do? Just try to rollback
		if err = tx.Rollback(ctx); err != nil {
			// nothing more to do to.
		}
	}
	return err
}

func (r *Repository2) SetCronId(ctx context.Context, tests []Test) error {
	return r.transactionUpdate(
		ctx,
		tests,
		func(test *Test, batch *pgx.Batch) {
			batch.Queue("update tests set cron_id = $2, updated_at = $3, where id = $1",
				test.Id, test.CronId, time.Now())
		},
	)
	//// create the batch of requests
	//var batch pgx.Batch
	//var result pgx.BatchResults
	//var err error
	//
	//defer func() {
	//	if err := result.Close(); err != nil {
	//		// log the error
	//	}
	//}()
	//
	//// start the transaction
	//tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	//if err != nil {
	//	return err
	//}
	//
	//// prepare the batch request
	//for _, test := range tests {
	//	batch.Queue("update tests set cron_id = $2, updated_at = $3, where id = $1",
	//		test.Id, test.CronId, time.Now())
	//}
	//
	//// now, send the batch requests
	//result = tx.SendBatch(ctx, &batch)
	//rows, err := result.Exec()
	//if err != nil {
	//	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
	//		// log the rollback error, there is nothing more we can do...
	//	}
	//	return err
	//}
	//if rows.RowsAffected() != int64(len(tests)) {
	//	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
	//		// log the rollback error, there is nothing more we can do...
	//	}
	//	return pgx.ErrNoRows
	//}
	//return err
}
