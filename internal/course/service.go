package course

import (
	"context"
	"strings"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) RecordAttempt(ctx context.Context, attempt Attempt) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := ValidateAttempt(attempt); err != nil {
		return err
	}

	if err := s.repo.Add(ctx, attempt); err != nil {
		return err
	}

	return nil
}

func (s *Service) Progress(ctx context.Context, learnerID, courseID string) (Progress, error) {
	if err := ctx.Err(); err != nil {
		return Progress{}, err
	}

	if strings.TrimSpace(learnerID) == "" {
		return Progress{}, ErrInvalidInput
	}

	if courseID != CourseID {
		return Progress{}, ErrUnknownCourse
	}

	attempts, err := s.repo.List(ctx, learnerID, courseID)
	if err != nil {
		return Progress{}, err
	}

	progress := Progress{
		LearnerID: learnerID,
		CourseID:  courseID,
	}

	mapLessonIDToScore := make(map[string]int)

	for _, attempt := range attempts {
		best, seen := mapLessonIDToScore[attempt.LessonID]
		if !seen || attempt.Score > best {
			mapLessonIDToScore[attempt.LessonID] = attempt.Score
		}
	}

	requiredLessonsPassed := 0
	requiredLessonsTotal := 0

	for _, lesson := range FixedCourse().Lessons {
		lessonProgress := LessonProgress{LessonID: lesson.ID}

		if bestScore, seen := mapLessonIDToScore[lesson.ID]; seen {
			lessonProgress.BestScore = &bestScore
			lessonProgress.Passed = bestScore >= lesson.PassingScore
		}

		progress.Lessons = append(progress.Lessons, lessonProgress)

		if lesson.Required {
			requiredLessonsTotal++
			if lessonProgress.Passed {
				requiredLessonsPassed++
			}
		}
	}

	progress.ProgressPercent = requiredLessonsPassed * 100 / requiredLessonsTotal
	progress.Completed = requiredLessonsPassed == requiredLessonsTotal

	return progress, nil
}
