package smallben

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/robfig/cron/v3"
	"time"
)

// To be used when dealing with generics.
type TestInfo interface {
	Id() int
	UserId() int
	CronId() int
	EverySecond() int
	Paused() bool
	CreatedAt() *time.Time
	UpdatedAt() *time.Time
	UserEvaluationRuleId() int
}

// Job is the struct used to interact with SmallBen.
type Job struct {
	// ID is a unique ID identifying the rawJob object.
	// It is chosen by the user.
	ID int64
	// GroupID is the ID of the group this rawJob is inserted in.
	GroupID int64
	// SuperGroupID specifies the ID of the super group
	// where this group is contained in.
	SuperGroupID int64
	// CronID is the ID of the cron rawJob as assigned by the scheduler
	// internally.
	CronID int64
	// EverySecond specifies every how many seconds the rawJob will run.
	EverySecond int64
	// Paused specifies whether this rawJob has been paused.
	Paused bool
	// createdAt specifies when this rawJob has been created.
	createdAt time.Time
	// updatedAt specifies the last time this object has been updated,
	// i.e., paused/resumed/schedule updated.
	updatedAt time.Time
	// Job is the unit of work to be executed
	Job CronJob
	// JobInput is the additional input to pass to the inner Job.
	JobInput map[string]interface{}
}

// Converts Job to a JobWithSchedule object.
func (j *Job) toJobWithSchedule() (JobWithSchedule, error) {
	var result JobWithSchedule
	// decode the schedule
	schedule, err := cron.ParseStandard(fmt.Sprintf("@every %ds", j.EverySecond))
	if err != nil {
		return result, err
	}

	result = JobWithSchedule{
		rawJob: RawJob{
			ID:           j.ID,
			GroupID:      j.GroupID,
			SuperGroupID: j.SuperGroupID,
			CronID:       0,
			EverySecond:  j.EverySecond,
			Paused:       false,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		},
		schedule: schedule,
		run:      j.Job,
		runInput: CronJobInput{
			JobID:        j.ID,
			GroupID:      j.GroupID,
			SuperGroupID: j.SuperGroupID,
			OtherInputs:  j.JobInput,
		},
	}
	return result, nil
}

// RawJob is the modelling a raw rawJob coming from the database.
type RawJob struct {
	// ID is a unique ID identifying the rawJob object.
	// It is chosen by the user.
	ID int64 `gorm:"primaryKey,column:id"`
	// GroupID is the ID of the group this rawJob is inserted in.
	GroupID int64 `gorm:"column:group_id"`
	// SuperGroupID specifies the ID of the super group
	// where this group is contained in.
	SuperGroupID int64 `gorm:"column:super_group_id"`
	// CronID is the ID of the cron rawJob as assigned by the scheduler
	// internally.
	CronID int64 `gorm:"column:cron_id"`
	// EverySecond specifies every how many seconds the rawJob will run.
	EverySecond int64 `gorm:"column:every_second"`
	// Paused specifies whether this rawJob has been paused.
	Paused bool `gorm:"column:paused"`
	// CreatedAt specifies when this rawJob has been created.
	CreatedAt time.Time `gorm:"column:created_at"`
	// UpdatedAt specifies the last time this object has been updated,
	// i.e., paused/resumed/schedule updated.
	UpdatedAt time.Time `gorm:"column:updated_at"`
	// SerializedJob is the gob-encoded byte array
	// of the interface executing this rawJob
	SerializedJob []byte `gorm:"column:serialized_job;type:bytea"`
	// SerializedJobInput is the gob-encoded byte array
	// of the input of the interface executing this rawJob
	SerializedJobInput []byte `gorm:"column:serialized_job_input;type:bytea"`
}

func (j *RawJob) TableName() string {
	return "jobs"
}

// JobWithSchedule is a rawJob object
// with a cron.Schedule object in it.
// The schedule can be accessed by using the Schedule()
// method.
// This object should be created only by calling the method
// ToJobWithSchedule().
type JobWithSchedule struct {
	rawJob   RawJob
	schedule cron.Schedule
	run      CronJob
	runInput CronJobInput
}

// Schedule returns the schedule used by this object.
func (j *JobWithSchedule) Schedule() *cron.Schedule {
	return &j.schedule
}

// ToJobWithSchedule returns a JobWithSchedule object from the current RawJob,
// by copy. It returns errors in case the given schedule is not valid,
// or in case the conversion of the rawJob interface/input fails.
func (j *RawJob) ToJobWithSchedule() (JobWithSchedule, error) {
	var result JobWithSchedule
	// decode the schedule
	schedule, err := cron.ParseStandard(fmt.Sprintf("@every %ds", j.EverySecond))
	if err != nil {
		return result, err
	}

	var decoder *gob.Decoder

	// decode the interface executing the rawJob
	decoder = gob.NewDecoder(bytes.NewBuffer(j.SerializedJob))
	var runJob CronJob
	if err = decoder.Decode(&runJob); err != nil {
		return result, err
	}

	// decode the input of the rawJob function
	decoder = gob.NewDecoder(bytes.NewBuffer(j.SerializedJobInput))
	var runJobInput CronJobInput
	if err = decoder.Decode(&runJobInput); err != nil {
		return result, err
	}

	result = JobWithSchedule{
		rawJob: RawJob{
			ID:           j.ID,
			GroupID:      j.GroupID,
			SuperGroupID: j.SuperGroupID,
			CronID:       j.CronID,
			EverySecond:  j.EverySecond,
			Paused:       j.Paused,
			CreatedAt:    j.CreatedAt,
			UpdatedAt:    j.UpdatedAt,
		},
		schedule: schedule,
		run:      runJob,
		runInput: runJobInput,
	}
	return result, nil
}

func (j *RawJob) schedule() (cron.Schedule, error) {
	return cron.ParseStandard(fmt.Sprintf("@every %ds", j.EverySecond))
}

func encodeJob(encoder *gob.Encoder, job CronJob) error {
	return encoder.Encode(&job)
}

// BuildJob builds the raw version of the inner job, by encoding
// the code to binary.
func (j *JobWithSchedule) BuildJob() (RawJob, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	// if err := encoder.Encode(&j.run); err != nil {
	//	return RawJob{}, err
	// }
	if err := encodeJob(encoder, j.run); err != nil {
		return RawJob{}, err
	}
	j.rawJob.SerializedJob = buffer.Bytes()
	buffer.Reset()
	if err := encoder.Encode(j.runInput); err != nil {
		return RawJob{}, err
	}
	j.rawJob.SerializedJobInput = buffer.Bytes()
	return j.rawJob, nil
}

// GetIdsFromJobRawList basically does jobs.map(rawJob -> rawJob.id)
func GetIdsFromJobRawList(jobs []RawJob) []int64 {
	ids := make([]int64, len(jobs))
	for i, test := range jobs {
		ids[i] = test.ID
	}
	return ids
}

// GetIdsFromJobsWithScheduleList basically does jobs.map(rawJob -> rawJob.id)
func GetIdsFromJobsWithScheduleList(jobs []JobWithSchedule) []int64 {
	ids := make([]int64, len(jobs))
	for i, job := range jobs {
		ids[i] = job.rawJob.ID
	}
	return ids
}

// GetIdsFromJobs basically does jobs.map(rawJob -> rawJob.id)
func GetIdsFromJobList(jobs []Job) []int64 {
	ids := make([]int64, len(jobs))
	for i, job := range jobs {
		ids[i] = job.ID
	}
	return ids
}

// UpdateSchedule is the struct used to update
// the schedule of a test.
type UpdateSchedule struct {
	// JobID is the ID of the tests
	JobID int64
	// EverySecond is the new schedule
	EverySecond int64
}

func (u *UpdateSchedule) schedule() (cron.Schedule, error) {
	return cron.ParseStandard(fmt.Sprintf("@every %ds", u.EverySecond))
}

// GetIdsFromUpdateScheduleList basically does schedules.map(rawJob -> rawJob.id)
func GetIdsFromUpdateScheduleList(schedules []UpdateSchedule) []int64 {
	ids := make([]int64, len(schedules))
	for i, test := range schedules {
		ids[i] = test.JobID
	}
	return ids
}

// CronJobInput is the input passed to the Run function.
type CronJobInput struct {
	JobID        int64
	GroupID      int64
	SuperGroupID int64
	OtherInputs  map[string]interface{}
}

// CronJob is the interface jobsToAdd has to implement.
// It contains only one single method, `Run`.
type CronJob interface {
	Run(input CronJobInput)
}
