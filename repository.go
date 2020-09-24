package smallben

import (
	"context"
	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"log"
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

// AddJobs add `jobs` within to the database. The update is done within a transaction.
func (r *Repository2) AddJobs(ctx context.Context, jobs []JobWithSchedule) error {
	rows := make([][]interface{}, len(jobs))
	for i, test := range jobs {
		rows[i] = test.addToRaw()
	}

	copyCount, err := r.pool.CopyFrom(ctx, pgx.Identifier{"jobs"}, addToColumn(), pgx.CopyFromRows(rows))
	if err != nil {
		return err
	}
	if copyCount != int64(len(jobs)) {
		return pgx.ErrNoRows
	}
	return nil
}

// GetJob returns a JobWithSchedule whose id is `jobID`.
// It returns an already converted JobWithSchedule.
func (r *Repository2) GetJob(ctx context.Context, jobID int32) (JobWithSchedule, error) {
	var jobs []Job
	err := pgxscan.Select(ctx, &r.pool, &jobs, `select id, group_id, super_group_id, cron_id,
every_second, paused, created_at, updated_at from jobs where id=$1`, jobID)
	if err != nil {
		return JobWithSchedule{}, err
	}
	if len(jobs) == 0 {
		return JobWithSchedule{}, pgx.ErrNoRows
	}
	job := jobs[0]
	jobWithSchedule, err := (&job).ToJobWithSchedule()
	return jobWithSchedule, err
}

// PauseJobs pauses `jobs`, i.e., changing the `paused` field to `true`.
func (r *Repository2) PauseJobs(ctx context.Context, jobs []Job) error {
	ids := GetIdsFromJobsList(jobs)
	_, err := r.pool.Exec(ctx, `update jobs set paused = true where id = any($1)`, ids)
	if err != nil {
		return err
	}
	return err
}

// ResumeJobs resumes `jobs`, i.e., changing the `paused` field to `false`.
func (r *Repository2) ResumeJobs(ctx context.Context, jobs []Job) error {
	ids := GetIdsFromJobsList(jobs)
	_, err := r.pool.Exec(ctx, `update jobs set paused = false where id = any($1)`, ids)
	if err != nil {
		return err
	}
	return nil
}

// GetAllJobsToExecute returns all the jobs whose `paused` field is set to `false`.
func (r *Repository2) GetAllJobsToExecute(ctx context.Context) ([]Job, error) {
	var jobs []Job
	err := pgxscan.Select(ctx, &r.pool, &jobs, `select id, group_id, super_group_id, cron_id,
every_second, paused, created_at, updated_at from jobs where paused = false`)
	return jobs, err
}

func (r *Repository2) GetAllJobsToExecute2(ctx context.Context) ([]JobWithSchedule, error){
	var rawJobs[] Job
	err := pgxscan.Select(ctx, &r.pool, &rawJobs, `select id, group_id, super_group_id, cron_id,
every_second, paused, created_at, updated_at from jobs where paused = false`)
	if err != nil {
		return nil, err
	}
	jobs := make([]JobWithSchedule, len(rawJobs))
	for i, rawJob := range rawJobs {
		job, err := rawJob.ToJobWithSchedule()
		if err != nil {
			return nil, err
		}
		jobs[i] = job
	}
	return jobs, nil
}

// GetJobsByIds returns all the jobs whose IDs are in `jobsID`. It returns an
// error of type `pgx.ErrNoRow` in case there is a mismatch between the length of the returned
// jobs and of the input.
func (r *Repository2) GetJobsByIds(ctx context.Context, jobsID []int32) ([]Job, error) {
	var jobs []Job
	err := pgxscan.Select(ctx, &r.pool, &jobs, `select select id, group_id, super_group_id, cron_id,
every_second, paused, created_at, updated_at from jobs where id = any($1)`, jobsID)
	if err != nil {
		return nil, err
	}
	if len(jobs) != len(jobsID) {
		return nil, pgx.ErrNoRows
	}
	return jobs, err
}

// DeleteJobsByIds deletes the jobs whose id are in `jobsID`.
func (r *Repository2) DeleteJobsByIds(ctx context.Context, jobsID []int32) error {
	_, err := r.pool.Exec(ctx, "delete from jobs where id = any($1)", jobsID)
	if err != nil {
		return err
	}
	return nil
}

//// ChangeSchedule update `tests` by saving in the database the new schedule.
//// Execution is done within a transaction.
//func (r *Repository2) ChangeSchedule(ctx context.Context, tests []Job) error {
//	return r.transactionUpdate(
//		ctx,
//		tests,
//		func(test *Job, batch *pgx.Batch) {
//			batch.Queue("update tests set every_second = $2, updated_at = $3 where id = $1",
//				test.ID, test.EverySecond, time.Now())
//		})
//}

//// ChangeScheduleOfTestsWithSchedule update `tests` by saving in the database the new schedule.
//// Execution is done within a transaction.
//func (r *Repository2) ChangeScheduleOfTestsWithSchedule(ctx context.Context, tests []JobWithSchedule) error {
//	rawTests := make([]Job, len(tests))
//	for i, test := range tests {
//		rawTests[i] = test.Job
//	}
//	return r.transactionUpdate(
//		ctx,
//		rawTests,
//		func(test *Job, batch *pgx.Batch) {
//			batch.Queue("update tests set every_second = $2, updated_at = $3 where id = $1",
//				test.ID, test.EverySecond, time.Now())
//		})
//}

// SetCronIdOfJobsWithSchedule updates `jobs` by updating the `CronID` field (and the `UpdatedAt`).
func (r *Repository2) SetCronIdOfJobsWithSchedule(ctx context.Context, jobs []JobWithSchedule) error {
	rawTests := make([]Job, len(jobs))
	for i, test := range jobs {
		rawTests[i] = test.Job
	}
	return r.transactionUpdate(
		ctx,
		rawTests,
		func(test *Job, batch *pgx.Batch) {
			batch.Queue("update jobs set cron_id = $2, updated_at = $3 where id = $1",
				test.ID, test.CronID, time.Now())
		},
	)
}

// SetCronIdAndChangeSchedule updates the `CronID` and the `EverySecond` field (and the `UpdatedAt`).
func (r *Repository2) SetCronIdAndChangeSchedule(ctx context.Context, jobs []JobWithSchedule) error {
	rawTests := make([]Job, len(jobs))
	for i, test := range jobs {
		rawTests[i] = test.Job
	}
	return r.transactionUpdate(
		ctx,
		rawTests,
		func(test *Job, batch *pgx.Batch) {
			batch.Queue("update jobs set cron_id = $2, every_second = $3 updated_at = $4 where id = $1",
				test.ID, test.CronID, test.EverySecond, time.Now())
		},
	)
}

func (r *Repository2) transactionUpdate(ctx context.Context, tests []Job, getBatchFn func(test *Job, batch *pgx.Batch)) error {
	// create the batch of requests
	var batch pgx.Batch
	var result pgx.BatchResults
	var err error

	// start the transaction
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	log.Printf("Begin transaction\n")

	// prepare the batch request
	for _, test := range tests {
		getBatchFn(&test, &batch)
	}

	// now, send the batch requests
	result = tx.SendBatch(ctx, &batch)
	_, err = result.Exec()
	if err != nil {
		log.Printf("Got error on result.Exec()")
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			log.Printf("Got error on tx.Rollback(), err: %s", err.Error())
			// log the rollback error, there is nothing more we can do...
		}
		return err
	}
	// before commit, I need to close the result
	if err = result.Close(); err != nil {
		// try rolling back
		log.Printf("Error on closing the result: %s\n", err.Error())
		if err = tx.Rollback(ctx); err != nil {
			log.Printf("Error on rollback: %s\n", err.Error())
			// nothing more to do to.
			return err
		}
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		log.Printf("Error on commit: %s\n", err.Error())
		// well, what do? Just try to rollback
		if err = tx.Rollback(ctx); err != nil {
			log.Printf("Error on rollback: %s\n", err.Error())
			// nothing more to do to.
			return err
		}
		return err
	}
	return nil
}
