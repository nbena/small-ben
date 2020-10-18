package smallben

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"github.com/robfig/cron/v3"
	"time"
)

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
	// cronID is the ID of the cron rawJob as assigned by the scheduler
	// internally.
	cronID int64
	// CronExpression specifies the scheduling of the job.
	CronExpression string
	// paused specifies whether this job has been paused.
	// Only used when returning this struct.
	paused bool
	// createdAt specifies when this rawJob has been created.
	createdAt time.Time
	// updatedAt specifies the last time this object has been updated,
	// i.e., paused/resumed/schedule updated.
	updatedAt time.Time
	// Job is the real unit of work to be executed
	Job CronJob
	// JobInput is the additional input to pass to the inner Job.
	JobInput map[string]interface{}
}

// toJobWithSchedule converts Job to a JobWithSchedule object.
// It returns an error in case the parsing of the cron expression fails.
func (j *Job) toJobWithSchedule() (JobWithSchedule, error) {
	var result JobWithSchedule
	// decode the schedule
	schedule, err := cron.ParseStandard(j.CronExpression)
	if err != nil {
		return result, err
	}

	result = JobWithSchedule{
		rawJob: RawJob{
			ID:             j.ID,
			GroupID:        j.GroupID,
			SuperGroupID:   j.SuperGroupID,
			CronID:         0,
			CronExpression: j.CronExpression,
			Paused:         false,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
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

// RawJob models a raw rawJob coming from the database.
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
	// CronExpression specifies the scheduling of the job.
	CronExpression string `gorm:"column:cron_expression"`
	// Paused specifies whether this rawJob has been paused.
	Paused bool `gorm:"column:paused"`
	// CreatedAt specifies when this rawJob has been created.
	CreatedAt time.Time `gorm:"column:created_at"`
	// UpdatedAt specifies the last time this object has been updated,
	// i.e., paused/resumed/schedule updated.
	UpdatedAt time.Time `gorm:"column:updated_at"`
	// SerializedJob is the base64(gob-encoded byte array)
	// of the interface executing this rawJob
	SerializedJob string `gorm:"column:serialized_job"`
	// SerializedJobInput is the base64(gob-encoded byte array)
	// of the map containing the argument for the job.
	SerializedJobInput string `gorm:"column:serialized_job_input"`
}

func (j *RawJob) TableName() string {
	return "jobs"
}

// JobWithSchedule is a RawJob object
// with a cron.Schedule object in it.
// The schedule can be accessed by using the Schedule() method.
// This object should be created only by calling the method
// toJobWithSchedule().
type JobWithSchedule struct {
	rawJob   RawJob
	schedule cron.Schedule
	run      CronJob
	runInput CronJobInput
}

// decodeSerializedFields decode j.serializedJob and j.SerializedJobInput.
func (j *RawJob) decodeSerializedFields() (CronJob, CronJobInput, error) {
	var decoder *gob.Decoder
	var err error

	// decode from base64 the serialized job
	decodedJob, err := base64.StdEncoding.DecodeString(j.SerializedJob)
	if err != nil {
		return nil, CronJobInput{}, err
	}

	// decode the interface executing the rawJob
	decoder = gob.NewDecoder(bytes.NewBuffer(decodedJob))
	var runJob CronJob
	if err = decoder.Decode(&runJob); err != nil {
		return nil, CronJobInput{}, err
	}

	// decode from base64 the serialized job input
	decodedJobInput, err := base64.StdEncoding.DecodeString(j.SerializedJobInput)
	if err != nil {
		return nil, CronJobInput{}, err
	}

	// decode the map containing the arguments will be passed
	// to the job
	decoder = gob.NewDecoder(bytes.NewBuffer(decodedJobInput))
	var jobInputMap map[string]interface{}
	// var runJobInput CronJobInput
	if err = decoder.Decode(&jobInputMap); err != nil {
		return nil, CronJobInput{}, err
	}
	// and build the overall object containing all the
	// inputs will be passed to the Job
	runJobInput := CronJobInput{
		JobID:        j.ID,
		GroupID:      j.GroupID,
		SuperGroupID: j.SuperGroupID,
		OtherInputs:  jobInputMap,
	}
	return runJob, runJobInput, nil
}

// toJob converts j to a Job instance.
func (j *RawJob) toJob() (Job, error) {
	job, jobInput, err := j.decodeSerializedFields()
	if err != nil {
		return Job{}, err
	}
	result := Job{
		ID:             j.ID,
		GroupID:        j.GroupID,
		SuperGroupID:   j.SuperGroupID,
		cronID:         j.CronID,
		CronExpression: j.CronExpression,
		paused:         j.Paused,
		createdAt:      j.CreatedAt,
		updatedAt:      j.UpdatedAt,
		Job:            job,
		JobInput:       jobInput.OtherInputs,
	}
	return result, nil
}

// ToJobWithSchedule returns a JobWithSchedule object from the current RawJob,
// by copy. It returns errors in case the given schedule is not valid,
// or in case the conversion of the rawJob interface/input fails.
// It does NOT copy the byte arrays from j.
func (j *RawJob) ToJobWithSchedule() (JobWithSchedule, error) {
	var result JobWithSchedule
	// decode the schedule
	schedule, err := cron.ParseStandard(j.CronExpression)
	if err != nil {
		return result, err
	}
	runJob, runJobInput, err := j.decodeSerializedFields()
	if err != nil {
		return result, err
	}

	result = JobWithSchedule{
		rawJob: RawJob{
			ID:             j.ID,
			GroupID:        j.GroupID,
			SuperGroupID:   j.SuperGroupID,
			CronID:         j.CronID,
			CronExpression: j.CronExpression,
			Paused:         j.Paused,
			CreatedAt:      j.CreatedAt,
			UpdatedAt:      j.UpdatedAt,
		},
		schedule: schedule,
		run:      runJob,
		runInput: runJobInput,
	}
	return result, nil
}

// encodeJob encodes `job`. A separate function is needed because we need to pass
// a POINTER to interface.
func encodeJob(encoder *gob.Encoder, job CronJob) error {
	return encoder.Encode(&job)
}

// BuildJob builds the raw version of the inner job, by encoding
// it to binary. This is needed since, when converting from a `RawJob` to a `JobWithSchedule`,
// the binary serialization of the Job is not kept in memory.
func (j *JobWithSchedule) BuildJob() (RawJob, error) {
	var bufferJob bytes.Buffer
	var bufferInput bytes.Buffer
	encoderJob := gob.NewEncoder(&bufferJob)
	// encode the CronJob interface keeping the unit of work
	// to execute. We need to use the encodeJob method
	// due to how gob interface encoding works.
	if err := encodeJob(encoderJob, j.run); err != nil {
		return RawJob{}, err
	}
	// finally, encode the bytes to base64
	j.rawJob.SerializedJob = base64.StdEncoding.EncodeToString(bufferJob.Bytes())

	// now, encode the job input
	// first, we build the decoded
	encoderInput := gob.NewEncoder(&bufferInput)
	// and finally, encode it
	if err := encoderInput.Encode(j.runInput.OtherInputs); err != nil {
		return RawJob{}, err
	}
	// finally, encode the bytes to base64
	j.rawJob.SerializedJobInput = base64.StdEncoding.EncodeToString(bufferInput.Bytes())
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
	// CronExpression is the new schedule
	CronExpression string
}

func (u *UpdateSchedule) schedule() (cron.Schedule, error) {
	return cron.ParseStandard(u.CronExpression)
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
