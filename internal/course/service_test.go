package course_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aksornproject/aksorn-senior-backend-assessment/internal/course"
	"github.com/aksornproject/aksorn-senior-backend-assessment/internal/infra"
)

func TestRecordAttemptRejectsUnknownLesson(t *testing.T) {
	// Invalid input is rejected before storage is accessed.
	service := course.NewService(nil)

	attempt := course.Attempt{
		AttemptID: "example-001",
		LearnerID: "learner-001",
		CourseID:  course.CourseID,
		LessonID:  "missing-lesson",
		Score:     75,
	}
	if err := service.RecordAttempt(
		context.Background(),
		attempt,
	); !errors.Is(
		err,
		course.ErrUnknownLesson,
	) {
		t.Fatalf("RecordAttempt error = %v, want ErrUnknownLesson", err)
	}
}

func TestProgressRejectsInvalidRequests(t *testing.T) {
	// Both checks run before storage is accessed, so no database is needed.
	service := course.NewService(nil)

	tests := []struct {
		name      string
		learnerID string
		courseID  string
		wantErr   error
	}{
		{
			name:      "blank learner",
			learnerID: "  ",
			courseID:  course.CourseID,
			wantErr:   course.ErrInvalidInput,
		},
		{
			name:      "unknown course",
			learnerID: "learner-001",
			courseID:  "course-999",
			wantErr:   course.ErrUnknownCourse,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.Progress(context.Background(), test.learnerID, test.courseID)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Progress error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestProgress(t *testing.T) {
	service := newTestService(t)

	tests := []struct {
		name          string
		attempts      []scoredAttempt
		wantLessons   map[string]lessonResult
		wantPercent   int
		wantCompleted bool
	}{
		{
			name:     "no attempts lists every lesson as not attempted",
			attempts: nil,
			wantLessons: map[string]lessonResult{
				"introduction": {bestScore: nil, passed: false},
				"practice":     {bestScore: nil, passed: false},
				"challenge":    {bestScore: nil, passed: false},
			},
			wantPercent:   0,
			wantCompleted: false,
		},
		{
			name:     "a real score of zero is shown as zero, not null",
			attempts: []scoredAttempt{{"practice", 0}},
			wantLessons: map[string]lessonResult{
				"introduction": {bestScore: nil, passed: false},
				"practice":     {bestScore: score(0), passed: false},
				"challenge":    {bestScore: nil, passed: false},
			},
			wantPercent:   0,
			wantCompleted: false,
		},
		{
			name:     "best score is the highest of several attempts",
			attempts: []scoredAttempt{{"introduction", 50}, {"introduction", 90}, {"introduction", 70}},
			wantLessons: map[string]lessonResult{
				"introduction": {bestScore: score(90), passed: true},
				"practice":     {bestScore: nil, passed: false},
				"challenge":    {bestScore: nil, passed: false},
			},
			wantPercent:   50,
			wantCompleted: false,
		},
		{
			name:     "a later lower score keeps the earlier pass",
			attempts: []scoredAttempt{{"practice", 75}, {"practice", 10}},
			wantLessons: map[string]lessonResult{
				"introduction": {bestScore: nil, passed: false},
				"practice":     {bestScore: score(75), passed: true},
				"challenge":    {bestScore: nil, passed: false},
			},
			wantPercent:   50,
			wantCompleted: false,
		},
		{
			// One point under each passing score: catches > used where >= is meant.
			name:     "one point below the passing score fails",
			attempts: []scoredAttempt{{"introduction", 59}, {"practice", 69}, {"challenge", 79}},
			wantLessons: map[string]lessonResult{
				"introduction": {bestScore: score(59), passed: false},
				"practice":     {bestScore: score(69), passed: false},
				"challenge":    {bestScore: score(79), passed: false},
			},
			wantPercent:   0,
			wantCompleted: false,
		},
		{
			name:     "exactly the passing score passes and completes the course",
			attempts: []scoredAttempt{{"introduction", 60}, {"practice", 70}, {"challenge", 80}},
			wantLessons: map[string]lessonResult{
				"introduction": {bestScore: score(60), passed: true},
				"practice":     {bestScore: score(70), passed: true},
				"challenge":    {bestScore: score(80), passed: true},
			},
			wantPercent:   100,
			wantCompleted: true,
		},
		{
			name:     "both required lessons complete the course without the optional one",
			attempts: []scoredAttempt{{"introduction", 100}, {"practice", 100}},
			wantLessons: map[string]lessonResult{
				"introduction": {bestScore: score(100), passed: true},
				"practice":     {bestScore: score(100), passed: true},
				"challenge":    {bestScore: nil, passed: false},
			},
			wantPercent:   100,
			wantCompleted: true,
		},
		{
			name:     "passing only the optional lesson adds no progress",
			attempts: []scoredAttempt{{"challenge", 100}},
			wantLessons: map[string]lessonResult{
				"introduction": {bestScore: nil, passed: false},
				"practice":     {bestScore: nil, passed: false},
				"challenge":    {bestScore: score(100), passed: true},
			},
			wantPercent:   0,
			wantCompleted: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			learnerID := uniqueLearnerID(t)
			recordAttempts(t, service, learnerID, test.attempts)

			got, err := service.Progress(context.Background(), learnerID, course.CourseID)
			if err != nil {
				t.Fatalf("Progress error = %v", err)
			}

			assertProgress(t, got, learnerID, test.wantLessons, test.wantPercent, test.wantCompleted)
		})
	}
}

func TestProgressIsolatesLearners(t *testing.T) {
	service := newTestService(t)

	finisher := uniqueLearnerID(t) + "-finisher"
	beginner := uniqueLearnerID(t) + "-beginner"

	recordAttempts(t, service, finisher, []scoredAttempt{{"introduction", 100}, {"practice", 100}})
	recordAttempts(t, service, beginner, []scoredAttempt{{"introduction", 30}})

	got, err := service.Progress(t.Context(), beginner, course.CourseID)
	if err != nil {
		t.Fatalf("Progress error = %v", err)
	}

	assertProgress(t, got, beginner, map[string]lessonResult{
		"introduction": {bestScore: score(30), passed: false},
		"practice":     {bestScore: nil, passed: false},
		"challenge":    {bestScore: nil, passed: false},
	}, 0, false)
}

type scoredAttempt struct {
	lessonID string
	score    int
}

// lessonResult is the expected result for one lesson; a nil bestScore means no attempt.
type lessonResult struct {
	bestScore *int
	passed    bool
}

func score(value int) *int { return &value }

// newTestService uses the PostgreSQL started by setup, through the supplied repository.
func newTestService(t *testing.T) *course.Service {
	t.Helper()

	connections, err := infra.Open(t.Context())
	if err != nil {
		t.Fatalf("open test database (run the README setup first): %v", err)
	}

	t.Cleanup(connections.Close)

	return course.NewService(course.NewRepository(connections.Postgres))
}

// uniqueLearnerID keeps runs independent: stored attempts are never deleted, so a
// learner ID reused from an earlier run would bring its old attempts along.
func uniqueLearnerID(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
}

func recordAttempts(t *testing.T, service *course.Service, learnerID string, attempts []scoredAttempt) {
	t.Helper()

	for i, attempt := range attempts {
		err := service.RecordAttempt(t.Context(), course.Attempt{
			AttemptID: fmt.Sprintf("%s-attempt-%d", learnerID, i),
			LearnerID: learnerID,
			CourseID:  course.CourseID,
			LessonID:  attempt.lessonID,
			Score:     attempt.score,
		})
		if err != nil {
			t.Fatalf("RecordAttempt(%s, %d) error = %v", attempt.lessonID, attempt.score, err)
		}
	}
}

// assertProgress compares lessons by ID, since the lesson order is not part of the contract.
func assertProgress(
	t *testing.T,
	got course.Progress,
	learnerID string,
	wantLessons map[string]lessonResult,
	wantPercent int,
	wantCompleted bool,
) {
	t.Helper()

	if got.LearnerID != learnerID || got.CourseID != course.CourseID {
		t.Errorf("progress for %q/%q, want %q/%q", got.LearnerID, got.CourseID, learnerID, course.CourseID)
	}

	if len(got.Lessons) != len(wantLessons) {
		t.Fatalf("got %d lessons, want %d: %+v", len(got.Lessons), len(wantLessons), got.Lessons)
	}

	seen := map[string]bool{}

	for _, lesson := range got.Lessons {
		want, known := wantLessons[lesson.LessonID]
		if !known || seen[lesson.LessonID] {
			t.Errorf("unexpected or repeated lesson %q", lesson.LessonID)

			continue
		}

		seen[lesson.LessonID] = true

		if formatScore(lesson.BestScore) != formatScore(want.bestScore) || lesson.Passed != want.passed {
			t.Errorf("%s: best_score=%s passed=%t, want best_score=%s passed=%t",
				lesson.LessonID, formatScore(lesson.BestScore), lesson.Passed,
				formatScore(want.bestScore), want.passed)
		}
	}

	if got.ProgressPercent != wantPercent {
		t.Errorf("progress_percent = %d, want %d", got.ProgressPercent, wantPercent)
	}

	if got.Completed != wantCompleted {
		t.Errorf("completed = %t, want %t", got.Completed, wantCompleted)
	}
}

// formatScore prints a best score the way the JSON shows it, so nil and 0 differ.
func formatScore(value *int) string {
	if value == nil {
		return "null"
	}

	return fmt.Sprint(*value)
}
